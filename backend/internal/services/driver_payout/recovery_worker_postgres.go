package driver_payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	postgresRecoveryLeaseDuration = 45 * time.Second
	postgresRecoveryCooldown      = 5 * time.Minute
	postgresRecoveryLookupTimeout = 30 * time.Second
	postgresRecoveryLeaseMargin   = 5 * time.Second
)

// PostgresPayoutRecoveryWorker performs explicitly invoked
// payout recovery using durable PostgreSQL coordination.
//
// It is never automatically scheduled and never initiates transfers.
//
// Financial finalization is disabled by default. When explicitly
// authorized, it requires independently verified provider success
// and a valid recovery lease.
type PostgresPayoutRecoveryWorker struct {
	db                           *pgxpool.Pool
	inspector                    PayoutRecoveryLookup
	verifiedRecovery             *LeaseFencedVerificationService
	financialFinalizationEnabled bool
	owner                        string
	staleAfter                   time.Duration
	batchSize                    int
	runGate                      chan struct{}
	// beforeLeaseValidation is a test-only synchronization hook.
	// It is nil during normal operation.
	beforeLeaseValidation func()
}

func NewPostgresPayoutRecoveryWorker(
	db *pgxpool.Pool,
	inspector PayoutRecoveryLookup,
	staleAfter time.Duration,
	batchSize int,
) (*PostgresPayoutRecoveryWorker, error) {
	if db == nil ||
		inspector == nil ||
		staleAfter <= 0 ||
		batchSize < 1 ||
		batchSize > maxPayoutRecoveryBatchSize {
		return nil, ErrInvalidRecovery
	}

	return &PostgresPayoutRecoveryWorker{
		db:         db,
		inspector:  inspector,
		owner:      uuid.NewString(),
		staleAfter: staleAfter,
		batchSize:  batchSize,
		runGate:    make(chan struct{}, 1),
	}, nil
}

// ConfigureVerifiedRecovery installs an independent, lease-fenced,
// read-only provider verification path.
//
// The existing inspector remains responsible for candidate discovery.
// This configuration does not schedule or start the worker.
func (w *PostgresPayoutRecoveryWorker) ConfigureVerifiedRecovery(
	verifier *LeaseFencedVerificationService,
) error {
	if w == nil || w.db == nil || verifier == nil ||
		verifier.db != w.db {
		return ErrInvalidRecovery
	}

	w.verifiedRecovery = verifier
	return nil
}

// ConfigureFinancialFinalization explicitly authorizes the worker
// to finalize independently verified successful provider transfers.
//
// This configuration does not schedule the worker or initiate transfers.
// The verified recovery service must already be configured.
//
// Financial finalization is disabled by default.
func (w *PostgresPayoutRecoveryWorker) ConfigureFinancialFinalization() error {
	if w == nil || w.db == nil || w.verifiedRecovery == nil ||
		w.verifiedRecovery.db != w.db {
		return ErrInvalidRecovery
	}

	w.financialFinalizationEnabled = true
	return nil
}

// RunOnce performs one bounded recovery cycle.
//
// Financial finalization requires explicit authorization and a
// configured lease-fenced verifier. Invalid authorization fails closed
// before candidate discovery.
//
// PostgreSQL leases coordinate independent worker processes.
// Cooldown begins at acquisition and survives worker restarts.
func (w *PostgresPayoutRecoveryWorker) RunOnce(
	ctx context.Context,
	onError func(error),
) error {
	if w == nil ||
		w.db == nil ||
		w.inspector == nil ||
		w.owner == "" ||
		w.runGate == nil ||
		w.staleAfter <= 0 ||
		w.batchSize < 1 ||
		w.batchSize > maxPayoutRecoveryBatchSize ||
		ctx == nil {
		return ErrInvalidRecovery
	}

	if w.financialFinalizationEnabled &&
		(w.verifiedRecovery == nil ||
			w.verifiedRecovery.db != w.db ||
			w.verifiedRecovery.verifier == nil) {
		return ErrInvalidRecovery
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case w.runGate <- struct{}{}:
		defer func() {
			<-w.runGate
		}()
	case <-ctx.Done():
		return ctx.Err()
	}

	candidates, err := w.inspector.DiscoverCandidates(
		ctx,
		time.Now().UTC().Add(-w.staleAfter),
		w.batchSize,
	)
	if err != nil {
		return fmt.Errorf(
			"discover payout recovery candidates: %w",
			err,
		)
	}

	if len(candidates) > w.batchSize {
		return errors.New(
			"payout recovery discovery exceeded batch limit",
		)
	}

	seen := make(map[string]struct{}, len(candidates))

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}

		if candidate.PayoutID == "" ||
			candidate.IdempotencyKey == "" {
			if onError != nil {
				onError(errors.New(
					"incomplete payout recovery identity",
				))
			}
			continue
		}

		if _, exists := seen[candidate.PayoutID]; exists {
			continue
		}
		seen[candidate.PayoutID] = struct{}{}

		lease, acquired, err :=
			postgresrepo.AcquireDriverPayoutRecoveryLease(
				ctx,
				w.db,
				candidate.PayoutID,
				w.owner,
				postgresRecoveryLeaseDuration,
				postgresRecoveryCooldown,
			)

		if err != nil {
			if onError != nil {
				onError(fmt.Errorf(
					"acquire recovery lease %s: %w",
					candidate.PayoutID,
					err,
				))
			}
			continue
		}

		if !acquired {
			continue
		}

		if lease == nil {
			if onError != nil {
				onError(errors.New(
					"recovery lease acquired without token",
				))
			}
			continue
		}

		// Reserve enough time for identity validation and the
		// provider lookup. Never contact the provider after the
		// authoritative lease deadline has become too close.
		leaseDeadline := lease.ExpiresAt.Add(
			-postgresRecoveryLeaseMargin,
		)
		lookupCtx, cancel := context.WithDeadline(
			ctx,
			leaseDeadline,
		)

		// Revalidate the complete authoritative execution identity
		// after lease acquisition and before provider contact.
		// A mismatch must never reach the provider.
		identityErr := postgresrepo.ValidateDriverPayoutRecoveryIdentity(
			lookupCtx,
			w.db,
			candidate,
		)

		var observation RecoveryObservation
		var lookupErr error

		if identityErr != nil {
			lookupErr = fmt.Errorf(
				"validate payout recovery identity: %w",
				identityErr,
			)
		} else if !recoveryLeaseAllowsLookup(
			leaseDeadline,
			time.Now(),
		) {
			lookupErr = errors.New(
				"insufficient recovery lease lifetime for provider lookup",
			)
		} else if err := lookupCtx.Err(); err != nil {
			lookupErr = err
		} else {
			if w.beforeLeaseValidation != nil {
				w.beforeLeaseValidation()
			}
			owned, ownershipErr :=
				postgresrepo.ValidateDriverPayoutRecoveryLease(
					lookupCtx,
					w.db,
					candidate.PayoutID,
					lease.Token,
					postgresRecoveryLookupTimeout+
						postgresRecoveryLeaseMargin,
				)

			if ownershipErr != nil {
				lookupErr = fmt.Errorf(
					"validate recovery lease ownership: %w",
					ownershipErr,
				)
			} else if !owned {
				lookupErr = errors.New(
					"recovery lease ownership expired or changed",
				)
			} else if err := lookupCtx.Err(); err != nil {
				lookupErr = err
			} else if !recoveryLeaseAllowsLookup(
				leaseDeadline,
				time.Now(),
			) {
				lookupErr = errors.New(
					"insufficient recovery lease lifetime for provider lookup",
				)
			} else {
				if w.verifiedRecovery != nil {
					verified, verificationErr :=
						w.verifiedRecovery.VerifyExistingTransfer(
							lookupCtx,
							candidate.PayoutID,
							lease.Token,
							postgresRecoveryLeaseMargin,
						)
					lookupErr = verificationErr

					if lookupErr == nil && w.financialFinalizationEnabled {
						if verified == nil {
							lookupErr = ErrAuthoritativeVerificationFailed
						} else {
							lookupErr = postgresrepo.FinalizeDriverPayoutRecoverySuccess(
								lookupCtx,
								w.db,
								candidate.PayoutID,
								candidate.Provider,
								verified.ProviderReference,
								lease.Token,
							)
						}
					}
				} else {
					observation, lookupErr =
						w.inspector.InspectCandidate(
							lookupCtx,
							candidate,
						)
				}
			}
		}

		lookupCtxErr := lookupCtx.Err()
		cancel()

		// Release must not depend on a canceled lookup context.
		releaseCtx, releaseCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)

		released, releaseErr :=
			postgresrepo.ReleaseDriverPayoutRecoveryLease(
				releaseCtx,
				w.db,
				candidate.PayoutID,
				lease.Token,
			)

		releaseCancel()

		if releaseErr != nil && onError != nil {
			onError(fmt.Errorf(
				"release recovery lease %s: %w",
				candidate.PayoutID,
				releaseErr,
			))
		}

		if releaseErr == nil && !released && onError != nil {
			onError(fmt.Errorf(
				"recovery lease ownership expired or changed: %s",
				candidate.PayoutID,
			))
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if lookupErr == nil && lookupCtxErr != nil {
			lookupErr = lookupCtxErr
		}

		if lookupErr == nil {
			lookupErr = observation.LookupError
		}

		if lookupErr != nil && onError != nil {
			onError(fmt.Errorf(
				"investigate driver payout %s: %w",
				candidate.PayoutID,
				lookupErr,
			))
		}

		// Provider observations are never financial finalization.
	}

	return nil
}

// recoveryLeaseAllowsLookup permits a provider lookup only when
// sufficient time remains before the lease safety deadline.
// The caller must also respect context cancellation.
func recoveryLeaseAllowsLookup(
	deadline time.Time,
	now time.Time,
) bool {
	return !now.Add(
		postgresRecoveryLookupTimeout,
	).After(deadline)
}
