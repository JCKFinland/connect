package flex_authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	stripego "github.com/stripe/stripe-go/v86"
)

var (
	ErrInvalidFlexWebhook = errors.New(
		"invalid FLEX authorization webhook",
	)
	ErrFlexWebhookNotReady = errors.New(
		"FLEX authorization is not ready for webhook reconciliation",
	)

	ErrFlexWebhookProcessing = errors.New(
		"FLEX webhook event is already being processed",
	)
)

// ReconcileWebhook retrieves the current authoritative Stripe PaymentIntent
// and reconciles a previously initiated FLEX authorization.
//
// The caller must first verify the Stripe webhook signature.
//
// This method never creates a PaymentIntent, captures funds,
// confirms an auction, or creates a trip.
func (s *Service) ReconcileWebhook(
	ctx context.Context,
	event *stripe.VerifiedFlexAuthorizationEvent,
) (*postgresrepo.FlexAuthorization, error) {
	if s == nil || s.db == nil || s.adapter == nil {
		return nil, ErrServiceUnavailable
	}

	if event == nil ||
		strings.TrimSpace(event.EventID) == "" ||
		strings.TrimSpace(event.AuthorizationID) == "" ||
		strings.TrimSpace(event.AuctionID) == "" ||
		strings.TrimSpace(event.OfferID) == "" ||
		strings.TrimSpace(event.PassengerID) == "" ||
		strings.TrimSpace(event.ProviderAuthorizationID) == "" {
		return nil, ErrInvalidFlexWebhook
	}

	// Resolve the authoritative local authorization in one query.
	// No authorization is created or claimed by webhook processing.
	var authorization postgresrepo.FlexAuthorization

	err := s.db.QueryRow(ctx, `
		SELECT
			id,
			auction_id,
			offer_id,
			passenger_id,
			ROUND(amount * 100)::bigint,
			currency,
			provider,
			idempotency_key,
			status,
			provider_authorization_id
		FROM flex_authorizations
		WHERE id = $1
	`, event.AuthorizationID).Scan(
		&authorization.ID,
		&authorization.AuctionID,
		&authorization.OfferID,
		&authorization.PassengerID,
		&authorization.AmountCents,
		&authorization.Currency,
		&authorization.Provider,
		&authorization.IdempotencyKey,
		&authorization.Status,
		&authorization.ProviderAuthorizationID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve FLEX webhook authorization: %w",
			err,
		)
	}

	if authorization.AuctionID != event.AuctionID ||
		authorization.OfferID != event.OfferID ||
		authorization.PassengerID != event.PassengerID ||
		authorization.AmountCents != event.AmountCents ||
		!strings.EqualFold(
			authorization.Currency,
			event.Currency,
		) ||
		!strings.EqualFold(
			authorization.Provider,
			"STRIPE",
		) ||
		event.CaptureMethod !=
			stripego.PaymentIntentCaptureMethodManual {
		return nil, ErrInvalidFlexWebhook
	}

	if authorization.ProviderAuthorizationID != nil &&
		*authorization.ProviderAuthorizationID !=
			event.ProviderAuthorizationID {
		return nil, ErrInvalidFlexWebhook
	}

	// Never let an unsolicited webhook initiate an authorization.
	// A prior application operation must have claimed PROCESSING.
	// A repeated capturable notification for the same already-authorized
	// PaymentIntent is idempotent. Never reopen a terminal authorization.
	if authorization.Status == "AUTHORIZED" {
		if authorization.ProviderAuthorizationID == nil ||
			*authorization.ProviderAuthorizationID != event.ProviderAuthorizationID ||
			event.EventType != "payment_intent.amount_capturable_updated" ||
			event.ProviderStatus != stripego.PaymentIntentStatusRequiresCapture {
			return nil, ErrInvalidFlexWebhook
		}

		return &authorization, nil
	}

	if authorization.Status != "PROCESSING" {
		return nil, ErrFlexWebhookNotReady
	}

	// Record the previously verified Stripe event before processing.
	// Registration is idempotent and rejects conflicting event identities.
	webhookEvent, _, err := postgresrepo.RegisterFlexWebhookEvent(
		ctx,
		s.db,
		postgresrepo.RegisterFlexWebhookEventRequest{
			ProviderEventID:         event.EventID,
			EventType:               event.EventType,
			AuthorizationID:         authorization.ID,
			ProviderAuthorizationID: event.ProviderAuthorizationID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"register FLEX webhook event: %w",
			err,
		)
	}

	// Only one worker may process this event at a time.
	claim, claimed, err := postgresrepo.ClaimFlexWebhookEventProcessing(
		ctx,
		s.db,
		webhookEvent.ID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"claim FLEX webhook event: %w",
			err,
		)
	}

	if !claimed {
		if claim != nil && claim.Status == "PROCESSED" {
			// This should be uncommon because terminal authorization
			// replay is handled above. Do not retrieve Stripe again.
			return nil, ErrFlexWebhookNotReady
		}

		return nil, ErrFlexWebhookProcessing
	}

	// Providing the known PaymentIntent ID forces the existing
	// Stripe adapter into retrieval mode, not creation mode.
	result, err := s.adapter.ExecuteFlexAuthorization(
		ctx,
		stripe.FlexAuthorizationRequest{
			AuthorizationID:         authorization.ID,
			AuctionID:               authorization.AuctionID,
			OfferID:                 authorization.OfferID,
			PassengerID:             authorization.PassengerID,
			AmountCents:             authorization.AmountCents,
			Currency:                authorization.Currency,
			IdempotencyKey:          authorization.IdempotencyKey,
			ProviderAuthorizationID: event.ProviderAuthorizationID,
		},
	)
	if err != nil {
		processingErr := fmt.Errorf(
			"retrieve authoritative FLEX PaymentIntent: %w",
			err,
		)

		return nil, s.releaseFailedWebhookClaim(
			ctx,
			webhookEvent.ID,
			claim.AttemptCount,
			"provider_retrieval_failed",
			processingErr,
		)
	}

	if result == nil ||
		result.ProviderAuthorizationID != event.ProviderAuthorizationID {
		return nil, s.releaseFailedWebhookClaim(
			ctx,
			webhookEvent.ID,
			claim.AttemptCount,
			"provider_identity_invalid",
			ErrInvalidFlexWebhook,
		)
	}

	var target postgresrepo.FlexAuthorizationResultStatus

	switch result.ProviderStatus {
	case stripego.PaymentIntentStatusRequiresCapture:
		if !result.Authorized ||
			result.RequiresCustomerAction {
			return nil, s.releaseFailedWebhookClaim(
				ctx,
				webhookEvent.ID,
				claim.AttemptCount,
				"provider_result_invalid",
				ErrInvalidFlexWebhook,
			)
		}
		target = postgresrepo.FlexResultAuthorized

	case stripego.PaymentIntentStatusCanceled:
		if result.Authorized ||
			result.RequiresCustomerAction {
			return nil, s.releaseFailedWebhookClaim(
				ctx,
				webhookEvent.ID,
				claim.AttemptCount,
				"provider_result_invalid",
				ErrInvalidFlexWebhook,
			)
		}
		target = postgresrepo.FlexResultCancelled

	case stripego.PaymentIntentStatusRequiresAction,
		stripego.PaymentIntentStatusRequiresConfirmation,
		stripego.PaymentIntentStatusRequiresPaymentMethod,
		stripego.PaymentIntentStatusProcessing:
		if result.Authorized {
			return nil, s.releaseFailedWebhookClaim(
				ctx,
				webhookEvent.ID,
				claim.AttemptCount,
				"provider_result_invalid",
				ErrInvalidFlexWebhook,
			)
		}
		target = postgresrepo.FlexResultProcessing

	default:
		return nil, s.releaseFailedWebhookClaim(
			ctx,
			webhookEvent.ID,
			claim.AttemptCount,
			"provider_result_invalid",
			ErrInvalidFlexWebhook,
		)
	}

	persisted, err := postgresrepo.CompleteFlexWebhookEvent(
		ctx,
		s.db,
		webhookEvent.ID,
		claim.AttemptCount,
		result.ProviderAuthorizationID,
		target,
	)
	if err != nil {
		processingErr := fmt.Errorf(
			"complete FLEX webhook reconciliation: %w",
			err,
		)

		return nil, s.releaseFailedWebhookClaim(
			ctx,
			webhookEvent.ID,
			claim.AttemptCount,
			"reconciliation_failed",
			processingErr,
		)
	}

	return persisted, nil
}

const flexWebhookFailureCleanupTimeout = 5 * time.Second

// releaseFailedWebhookClaim records a safe diagnostic and releases
// the current processing attempt for immediate retry.
//
// Cleanup is independent of request cancellation but has a strict
// timeout. The repository fences updates by attempt and lease.
//
// The original processing error is always preserved.
func (s *Service) releaseFailedWebhookClaim(
	ctx context.Context,
	eventID string,
	attempt int,
	diagnostic string,
	processingErr error,
) error {
	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		flexWebhookFailureCleanupTimeout,
	)
	defer cancel()

	_, releaseErr := postgresrepo.FailFlexWebhookEvent(
		cleanupCtx,
		s.db,
		eventID,
		attempt,
		diagnostic,
	)

	if releaseErr != nil {
		return errors.Join(
			processingErr,
			fmt.Errorf(
				"release failed FLEX webhook claim: %w",
				releaseErr,
			),
		)
	}

	return processingErr
}
