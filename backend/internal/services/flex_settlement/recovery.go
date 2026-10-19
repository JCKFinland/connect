package flex_settlement

import (
	"context"
	"errors"
	"fmt"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	stripego "github.com/stripe/stripe-go/v86"
)

type RecoveryStatus string

const (
	RecoveryReadyForCapture  RecoveryStatus = "READY_FOR_CAPTURE"
	RecoveryCaptured         RecoveryStatus = "CAPTURED"
	RecoveryPendingProvider  RecoveryStatus = "PENDING_PROVIDER"
	RecoveryCancelled        RecoveryStatus = "CANCELLED"
	RecoveryAlreadyFinalized RecoveryStatus = "ALREADY_FINALIZED"
)

type RecoveryResult struct {
	SettlementID      string
	Status            RecoveryStatus
	ProviderCaptureID string
	ProviderStatus    stripego.PaymentIntentStatus
}

type RecoveryService struct {
	db       *pgxpool.Pool
	verifier stripepayment.FlexCaptureVerifier
}

func NewRecoveryService(
	db *pgxpool.Pool,
	verifier stripepayment.FlexCaptureVerifier,
) *RecoveryService {
	return &RecoveryService{
		db:       db,
		verifier: verifier,
	}
}

// Recover inspects authoritative database and Stripe state.
//
// It does not claim a settlement, create financial records,
// capture funds, or finalize a payment.
func (s *RecoveryService) Recover(
	ctx context.Context,
	settlementID string,
) (*RecoveryResult, error) {
	if s == nil || s.db == nil || s.verifier == nil {
		return nil, errors.New(
			"FLEX capture recovery dependencies are not configured",
		)
	}

	if _, err := uuid.Parse(settlementID); err != nil {
		return nil, postgres.ErrFlexSettlementNotFound
	}

	var (
		authorizationID         string
		auctionID               string
		offerID                 string
		passengerID             string
		providerAuthorizationID string
		provider                string
		settlementStatus        string
		currency                string
		amount                  string
	)

	err := s.db.QueryRow(ctx, `
        SELECT
            fs.authorization_id,
            fa.auction_id,
            fa.offer_id,
            fa.passenger_id,
            fs.provider_authorization_id,
            fs.provider,
            fs.status,
            fs.currency,
            fs.amount::text
        FROM flex_settlements fs
        JOIN flex_authorizations fa
          ON fa.id = fs.authorization_id
        WHERE fs.id = $1
          AND fa.status = 'AUTHORIZED'
          AND fa.provider = fs.provider
          AND fa.provider_authorization_id =
              fs.provider_authorization_id
          AND fa.amount = fs.amount
          AND fa.currency = fs.currency
    `, settlementID).Scan(
		&authorizationID,
		&auctionID,
		&offerID,
		&passengerID,
		&providerAuthorizationID,
		&provider,
		&settlementStatus,
		&currency,
		&amount,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, postgres.ErrFlexSettlementNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"read FLEX settlement recovery identity: %w",
			err,
		)
	}

	if provider != "STRIPE" {
		return nil, errors.New(
			"unsupported FLEX settlement provider",
		)
	}

	result := &RecoveryResult{
		SettlementID: settlementID,
	}

	switch settlementStatus {
	case "CAPTURED":
		result.Status = RecoveryAlreadyFinalized
		return result, nil

	case "CANCELLED":
		result.Status = RecoveryCancelled
		return result, nil

	case "PENDING", "PROCESSING":
		// Both states require authoritative provider verification.

	default:
		return nil, postgres.ErrFlexSettlementInvalidState
	}

	cents, err := postgresFlexAmountToCents(amount)
	if err != nil {
		return nil, fmt.Errorf(
			"decode FLEX recovery amount: %w",
			err,
		)
	}

	verification, err := s.verifier.VerifyFlexCapture(
		ctx,
		stripepayment.FlexCaptureVerificationRequest{
			ProviderAuthorizationID: providerAuthorizationID,
			AuthorizationID:         authorizationID,
			AuctionID:               auctionID,
			OfferID:                 offerID,
			PassengerID:             passengerID,
			AmountCents:             cents,
			Currency:                currency,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"verify FLEX settlement provider state: %w",
			err,
		)
	}
	if verification == nil {
		return nil, errors.New(
			"FLEX capture verifier returned no result",
		)
	}

	result.ProviderCaptureID = verification.ProviderCaptureID
	result.ProviderStatus = verification.ProviderStatus

	switch {
	case verification.Captured:
		result.Status = RecoveryCaptured

	case verification.ProviderStatus ==
		stripego.PaymentIntentStatusRequiresCapture:
		if settlementStatus == "PROCESSING" {
			result.Status = RecoveryReadyForCapture
		} else {
			result.Status = RecoveryPendingProvider
		}

	default:
		result.Status = RecoveryPendingProvider
	}

	return result, nil
}
