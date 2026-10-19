package trip

import (
	"context"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

type flexCaptureExecutionFakeProvider struct {
	db interface {
		QueryRow(context.Context, string, ...any) interface{}
	}
}

type recordingFlexCaptureExecutor struct {
	calls   int
	request stripepayment.FlexCaptureRequest
	result  *stripepayment.FlexCaptureResult
	err     error
	before  func(context.Context) error
}

func (e *recordingFlexCaptureExecutor) ExecuteFlexCapture(
	ctx context.Context,
	request stripepayment.FlexCaptureRequest,
) (*stripepayment.FlexCaptureResult, error) {
	e.calls++
	e.request = request

	if e.before != nil {
		if err := e.before(ctx); err != nil {
			return nil, err
		}
	}

	return e.result, e.err
}

type statefulFlexCaptureVerifier struct {
	calls      int
	captured   bool
	providerID string
}

func (v *statefulFlexCaptureVerifier) VerifyFlexCapture(
	_ context.Context,
	request stripepayment.FlexCaptureVerificationRequest,
) (*stripepayment.FlexCaptureVerificationResult, error) {
	v.calls++

	if request.ProviderAuthorizationID != v.providerID {
		return nil, postgres.ErrFlexSettlementInvalidState
	}

	if v.captured {
		return &stripepayment.FlexCaptureVerificationResult{
			ProviderCaptureID: v.providerID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
			Captured:          true,
		}, nil
	}

	return &stripepayment.FlexCaptureVerificationResult{
		ProviderCaptureID: v.providerID,
		ProviderStatus:    stripego.PaymentIntentStatusRequiresCapture,
		Captured:          false,
	}, nil
}

func TestFlexCaptureExecutionSuccessfulLifecycle(t *testing.T) {
	db, settlementID := prepareFlexCaptureMarkerFixture(t)
	ctx := context.Background()

	// Return this prepared fixture to the initial PENDING state
	// is not permitted. Instead, test the legitimate PROCESSING
	// lifecycle that already has financial records prepared.
	var providerID string
	var amountCents int64
	var currency string
	var captureKey string
	var tripID string

	err := db.QueryRow(ctx, `
		SELECT
			provider_authorization_id,
			(amount * 100)::bigint,
			currency,
			capture_idempotency_key,
			trip_id
		FROM flex_settlements
		WHERE id = $1
	`, settlementID).Scan(
		&providerID,
		&amountCents,
		&currency,
		&captureKey,
		&tripID,
	)
	if err != nil {
		t.Fatal(err)
	}

	verifier := &statefulFlexCaptureVerifier{
		providerID: providerID,
	}

	executor := &recordingFlexCaptureExecutor{
		result: &stripepayment.FlexCaptureResult{
			ProviderCaptureID: providerID,
			ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
		},
	}

	executor.before = func(ctx context.Context) error {
		var markerExists bool

		err := db.QueryRow(ctx, `
			SELECT capture_requested_at IS NOT NULL
			FROM flex_settlements
			WHERE id = $1
		`, settlementID).Scan(&markerExists)
		if err != nil {
			return err
		}
		if !markerExists {
			return postgres.ErrFlexSettlementInvalidState
		}

		verifier.captured = true
		return nil
	}

	orchestrator := flexsettlement.NewOrchestrator(
		db,
		flexsettlement.NewRecoveryService(db, verifier),
	)

	result, err := orchestrator.ExecuteCapture(
		ctx,
		settlementID,
		executor,
	)
	if err != nil {
		t.Fatalf("execute FLEX capture: %v", err)
	}

	if result == nil ||
		result.Status != flexsettlement.RecoveryCaptured {
		t.Fatalf("unexpected capture result: %+v", result)
	}

	if executor.calls != 1 {
		t.Fatalf("expected one capture call, got %d", executor.calls)
	}

	if verifier.calls != 2 {
		t.Fatalf(
			"expected pre- and post-capture verification, got %d",
			verifier.calls,
		)
	}

	if executor.request.ProviderAuthorizationID != providerID ||
		executor.request.AmountCents != amountCents ||
		executor.request.Currency != currency ||
		executor.request.CaptureIdempotencyKey != captureKey {
		t.Fatalf("capture request identity mismatch: %+v", executor.request)
	}

	assertCapturedFlexOrchestratorState(
		t,
		ctx,
		db,
		settlementID,
		tripID,
		providerID,
		amountCents,
	)

	replay, err := orchestrator.ExecuteCapture(
		ctx,
		settlementID,
		executor,
	)
	if err != nil {
		t.Fatalf("capture replay: %v", err)
	}

	if replay == nil ||
		replay.Status != flexsettlement.RecoveryAlreadyFinalized {
		t.Fatalf("unexpected replay result: %+v", replay)
	}

	if executor.calls != 1 {
		t.Fatalf(
			"replay submitted another capture request: %d",
			executor.calls,
		)
	}
}
