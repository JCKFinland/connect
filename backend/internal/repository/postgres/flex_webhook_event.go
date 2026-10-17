package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrFlexWebhookEventInvalid = errors.New(
		"invalid FLEX webhook event",
	)

	ErrFlexWebhookEventIdentityConflict = errors.New(
		"FLEX webhook event identity conflict",
	)
)

type FlexWebhookEvent struct {
	ID                      string
	Provider                string
	ProviderEventID         string
	EventType               string
	AuthorizationID         string
	ProviderAuthorizationID string
	Status                  string
	AttemptCount            int
	LastError               *string
	ReceivedAt              time.Time
	ProcessingStartedAt     *time.Time
	ProcessedAt             *time.Time
	UpdatedAt               time.Time
}

type RegisterFlexWebhookEventRequest struct {
	ProviderEventID         string
	EventType               string
	AuthorizationID         string
	ProviderAuthorizationID string
}

const flexWebhookEventColumns = `
	id,
	provider,
	provider_event_id,
	event_type,
	authorization_id,
	provider_authorization_id,
	status,
	attempt_count,
	last_error,
	received_at,
	processing_started_at,
	processed_at,
	updated_at
`

func scanFlexWebhookEvent(row pgx.Row) (*FlexWebhookEvent, error) {
	var event FlexWebhookEvent

	err := row.Scan(
		&event.ID,
		&event.Provider,
		&event.ProviderEventID,
		&event.EventType,
		&event.AuthorizationID,
		&event.ProviderAuthorizationID,
		&event.Status,
		&event.AttemptCount,
		&event.LastError,
		&event.ReceivedAt,
		&event.ProcessingStartedAt,
		&event.ProcessedAt,
		&event.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

// RegisterFlexWebhookEvent durably records a verified Stripe event.
//
// Repeated registration returns the existing event only when all
// immutable event identity fields match.
//
// Registration never marks an event processed and never modifies
// an existing event's processing state.
func RegisterFlexWebhookEvent(
	ctx context.Context,
	db *pgxpool.Pool,
	request RegisterFlexWebhookEventRequest,
) (*FlexWebhookEvent, bool, error) {
	if db == nil {
		return nil, false, ErrFlexWebhookEventInvalid
	}

	request.ProviderEventID = strings.TrimSpace(
		request.ProviderEventID,
	)
	request.EventType = strings.TrimSpace(request.EventType)
	request.AuthorizationID = strings.TrimSpace(
		request.AuthorizationID,
	)
	request.ProviderAuthorizationID = strings.TrimSpace(
		request.ProviderAuthorizationID,
	)

	if request.ProviderEventID == "" ||
		request.EventType == "" ||
		request.ProviderAuthorizationID == "" {
		return nil, false, ErrFlexWebhookEventInvalid
	}

	if _, err := uuid.Parse(request.AuthorizationID); err != nil {
		return nil, false, ErrFlexWebhookEventInvalid
	}

	var event *FlexWebhookEvent
	var created bool

	err := RunInTransaction(
		ctx,
		db,
		func(tx pgx.Tx) error {
			eventID := ""

			err := tx.QueryRow(ctx, `
				INSERT INTO flex_webhook_events (
					provider,
					provider_event_id,
					event_type,
					authorization_id,
					provider_authorization_id
				)
				VALUES (
					'STRIPE',
					$1,
					$2,
					$3,
					$4
				)
				ON CONFLICT (provider, provider_event_id)
				DO NOTHING
				RETURNING id
			`,
				request.ProviderEventID,
				request.EventType,
				request.AuthorizationID,
				request.ProviderAuthorizationID,
			).Scan(&eventID)

			switch {
			case err == nil:
				created = true

			case errors.Is(err, pgx.ErrNoRows):
				created = false

			default:
				return fmt.Errorf(
					"register FLEX webhook event: %w",
					err,
				)
			}

			event, err = scanFlexWebhookEvent(
				tx.QueryRow(ctx, `
					SELECT `+flexWebhookEventColumns+`
					FROM flex_webhook_events
					WHERE provider = 'STRIPE'
					  AND provider_event_id = $1
					FOR UPDATE
				`, request.ProviderEventID),
			)
			if err != nil {
				return fmt.Errorf(
					"load FLEX webhook event: %w",
					err,
				)
			}

			if event.EventType != request.EventType ||
				event.AuthorizationID != request.AuthorizationID ||
				event.ProviderAuthorizationID !=
					request.ProviderAuthorizationID {
				return ErrFlexWebhookEventIdentityConflict
			}

			return nil
		},
	)
	if err != nil {
		return nil, false, err
	}

	return event, created, nil
}
