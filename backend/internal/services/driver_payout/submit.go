package driver_payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidSubmission = errors.New(
		"invalid driver payout submission",
	)
	ErrSubmissionNotAuthorized = errors.New(
		"driver payout submission not authorized",
	)
	ErrSubmissionUnresolved = errors.New(
		"driver payout submission outcome unresolved",
	)
)

type Submitter struct {
	db       *pgxpool.Pool
	provider payouts.Provider
}

func NewSubmitter(
	db *pgxpool.Pool,
	provider payouts.Provider,
) (*Submitter, error) {
	if db == nil || provider == nil {
		return nil, ErrInvalidSubmission
	}

	return &Submitter{
		db:       db,
		provider: provider,
	}, nil
}

// SubmitPayout authorizes at most one provider initiation.
//
// A previously PROCESSING execution is never submitted again.
// All provider outcomes require independent reconciliation before
// successful financial finalization.
func (s *Submitter) SubmitPayout(
	ctx context.Context,
	payoutID string,
) error {
	if s == nil || s.db == nil || s.provider == nil {
		return ErrInvalidSubmission
	}

	execution, claimed, err :=
		postgresrepo.ClaimDriverPayoutExecution(
			ctx, s.db, payoutID,
		)
	if err != nil {
		return fmt.Errorf(
			"claim payout submission: %w", err,
		)
	}

	if !claimed || execution == nil {
		return ErrSubmissionNotAuthorized
	}

	request, err :=
		postgresrepo.ResolveDriverPayoutTransferRequest(
			ctx, s.db, payoutID,
		)
	if err != nil {
		// Claim was committed. We cannot safely authorize a retry.
		return s.markUnresolved(
			ctx, payoutID,
			"unable to resolve authoritative transfer request",
			err,
		)
	}

	if request.IdempotencyKey != execution.IdempotencyKey ||
		request.PayoutID != execution.PayoutID {
		return s.markUnresolved(
			ctx, payoutID,
			"claimed execution does not match transfer request",
			ErrSubmissionNotAuthorized,
		)
	}

	result, err := s.provider.InitiateTransfer(
		ctx, *request,
	)

	if err != nil {
		return s.markUnresolved(
			ctx, payoutID,
			"provider initiation outcome unknown",
			err,
		)
	}

	if err := payouts.ValidateTransferResult(result); err != nil {
		return s.markUnresolved(
			ctx, payoutID,
			"invalid provider initiation response",
			err,
		)
	}

	// Even a reported success is not authoritative financial proof.
	// A separate lookup and independent verification are required.
	return s.markUnresolved(
		ctx, payoutID,
		"provider submission requires independent reconciliation",
		ErrSubmissionUnresolved,
	)
}

func (s *Submitter) markUnresolved(
	ctx context.Context,
	payoutID string,
	reason string,
	cause error,
) error {
	// A cancelled request context must not prevent recording
	// the uncertain financial state.
	recordCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		10*time.Second,
	)
	defer cancel()

	_, markErr :=
		postgresrepo.MarkDriverPayoutRequiresReconciliation(
			recordCtx,
			s.db,
			payoutID,
			reason,
		)

	if markErr != nil {
		return fmt.Errorf(
			"%w: %v; reconciliation state update failed: %v",
			ErrSubmissionUnresolved,
			cause,
			markErr,
		)
	}

	return fmt.Errorf(
		"%w: %v",
		ErrSubmissionUnresolved,
		cause,
	)
}
