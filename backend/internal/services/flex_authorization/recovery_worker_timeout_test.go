package flex_authorization

import (
	"context"
	"errors"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

type authorizationTimeoutExecutor struct {
	run func(context.Context, string) (*RecoveryReconcileResult, error)
}

func (e authorizationTimeoutExecutor) RecoverAndReconcile(
	ctx context.Context,
	id string,
) (*RecoveryReconcileResult, error) {
	return e.run(ctx, id)
}

func newAuthorizationTimeoutWorker(
	t *testing.T,
	ids []string,
	executor AuthorizationRecoveryExecutor,
) *AuthorizationRecoveryWorker {
	t.Helper()

	candidates := make(
		[]postgresrepo.FlexAuthorizationRecoveryCandidate,
		0,
		len(ids),
	)

	for _, id := range ids {
		candidates = append(
			candidates,
			postgresrepo.FlexAuthorizationRecoveryCandidate{
				AuthorizationID: id,
			},
		)
	}

	worker, err := NewAuthorizationRecoveryWorker(
		&authorizationWorkerDiscoveryFake{
			candidates: candidates,
		},
		executor,
		5*time.Minute,
		len(ids),
	)
	if err != nil {
		t.Fatal(err)
	}

	return worker
}

func TestAuthorizationRecoveryTimeoutPropagatesDeadline(
	t *testing.T,
) {
	var observedDeadline time.Time

	executor := authorizationTimeoutExecutor{
		run: func(
			ctx context.Context,
			id string,
		) (*RecoveryReconcileResult, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				return nil, errors.New("missing recovery deadline")
			}

			observedDeadline = deadline

			return &RecoveryReconcileResult{
				AuthorizationID: id,
				Status:          "AUTHORIZED",
			}, nil
		},
	}

	worker := newAuthorizationTimeoutWorker(
		t,
		[]string{"authorization"},
		executor,
	)

	before := time.Now()

	if err := worker.RunOnce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	after := time.Now()

	minimum := before.Add(defaultAuthorizationRecoveryAttemptTimeout)
	maximum := after.Add(defaultAuthorizationRecoveryAttemptTimeout)

	if observedDeadline.Before(minimum) ||
		observedDeadline.After(maximum) {
		t.Fatalf(
			"unexpected recovery deadline: %v",
			observedDeadline,
		)
	}
}

func TestAuthorizationRecoveryTimeoutPreservesEarlierParentDeadline(
	t *testing.T,
) {
	parent, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	parentDeadline, _ := parent.Deadline()

	executor := authorizationTimeoutExecutor{
		run: func(
			ctx context.Context,
			id string,
		) (*RecoveryReconcileResult, error) {
			deadline, ok := ctx.Deadline()
			if !ok || !deadline.Equal(parentDeadline) {
				return nil, errors.New(
					"parent deadline was not preserved",
				)
			}

			return &RecoveryReconcileResult{
				AuthorizationID: id,
				Status:          "AUTHORIZED",
			}, nil
		},
	}

	worker := newAuthorizationTimeoutWorker(
		t,
		[]string{"authorization"},
		executor,
	)

	var reported []error

	err := worker.RunOnce(parent, func(err error) {
		reported = append(reported, err)
	})

	if err != nil {
		t.Fatal(err)
	}

	if len(reported) != 0 {
		t.Fatalf("unexpected recovery errors: %v", reported)
	}
}

func TestAuthorizationRecoveryTimeoutContinuesHealthyCandidates(
	t *testing.T,
) {
	var executed []string

	executor := authorizationTimeoutExecutor{
		run: func(
			ctx context.Context,
			id string,
		) (*RecoveryReconcileResult, error) {
			executed = append(executed, id)

			if id == "timed-out" {
				return nil, context.DeadlineExceeded
			}

			return &RecoveryReconcileResult{
				AuthorizationID: id,
				Status:          "AUTHORIZED",
			}, nil
		},
	}

	worker := newAuthorizationTimeoutWorker(
		t,
		[]string{"timed-out", "healthy"},
		executor,
	)

	var reported []error

	err := worker.RunOnce(
		context.Background(),
		func(err error) {
			reported = append(reported, err)
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(executed) != 2 ||
		executed[0] != "timed-out" ||
		executed[1] != "healthy" {
		t.Fatalf("unexpected execution order: %v", executed)
	}

	if len(reported) != 1 ||
		!errors.Is(reported[0], context.DeadlineExceeded) {
		t.Fatalf("unexpected timeout reports: %v", reported)
	}

	if len(worker.cooldowns) != 1 {
		t.Fatal("timed-out candidate was not cooled down")
	}
}

func TestAuthorizationRecoveryTimeoutRejectsLateSuccess(
	t *testing.T,
) {
	executor := authorizationTimeoutExecutor{
		run: func(
			ctx context.Context,
			id string,
		) (*RecoveryReconcileResult, error) {
			<-ctx.Done()

			return &RecoveryReconcileResult{
				AuthorizationID: id,
				Status:          "AUTHORIZED",
			}, nil
		},
	}

	worker := newAuthorizationTimeoutWorker(
		t,
		[]string{"late-success"},
		executor,
	)

	parent, cancel := context.WithTimeout(
		context.Background(),
		20*time.Millisecond,
	)
	defer cancel()

	err := worker.RunOnce(parent, nil)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected parent deadline, got %v", err)
	}
}

func TestAuthorizationRecoveryTimeoutStopsOnParentCancellation(
	t *testing.T,
) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	var executed []string

	executor := authorizationTimeoutExecutor{
		run: func(
			ctx context.Context,
			id string,
		) (*RecoveryReconcileResult, error) {
			executed = append(executed, id)

			cancel()

			<-ctx.Done()

			return nil, ctx.Err()
		},
	}

	worker := newAuthorizationTimeoutWorker(
		t,
		[]string{"first", "second"},
		executor,
	)

	err := worker.RunOnce(parent, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	if len(executed) != 1 || executed[0] != "first" {
		t.Fatalf(
			"worker continued after cancellation: %v",
			executed,
		)
	}
}
