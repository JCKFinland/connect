package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrFlexSettlementNotEligible = errors.New(
		"FLEX settlement is not eligible",
	)
	ErrFlexSettlementNotFound = errors.New(
		"FLEX settlement not found",
	)
)

type FlexSettlement struct {
	ID                      string
	TripID                  string
	AuthorizationID         string
	AmountCents             int64
	Currency                string
	Provider                string
	ProviderAuthorizationID string
	CaptureIdempotencyKey   string
	Status                  string
	AttemptCount            int
}

const flexSettlementColumns = `
	id,
	trip_id,
	authorization_id,
	amount::text,
	currency,
	provider,
	provider_authorization_id,
	capture_idempotency_key,
	status,
	attempt_count
`

func scanFlexSettlement(row pgx.Row) (*FlexSettlement, error) {
	s := &FlexSettlement{}
	var amount string

	err := row.Scan(
		&s.ID,
		&s.TripID,
		&s.AuthorizationID,
		&amount,
		&s.Currency,
		&s.Provider,
		&s.ProviderAuthorizationID,
		&s.CaptureIdempotencyKey,
		&s.Status,
		&s.AttemptCount,
	)
	if err != nil {
		return nil, err
	}

	s.AmountCents, err = flexDecimalToCents(amount)
	if err != nil {
		return nil, fmt.Errorf(
			"decode FLEX settlement amount: %w",
			err,
		)
	}

	return s, nil
}

// CreateFlexSettlementFromCompletedTrip creates the immutable
// settlement identity for a completed, authorized FLEX trip.
//
// The database derives the amount, currency, provider, and
// authorization identity. No payment provider is contacted.
//
// Repeated calls for the same trip return the existing settlement.
func CreateFlexSettlementFromCompletedTrip(
	ctx context.Context,
	db *pgxpool.Pool,
	tripID string,
) (*FlexSettlement, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX settlement database is required",
		)
	}

	var result *FlexSettlement

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var err error

		result, err = CreateFlexSettlementFromCompletedTripTx(
			ctx,
			tx,
			tripID,
		)

		return err
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// CreateFlexSettlementFromCompletedTripTx creates or retrieves
// the settlement using the caller's PostgreSQL transaction.
//
// It does not begin, commit, or roll back the transaction.
// No external payment provider is contacted.
func CreateFlexSettlementFromCompletedTripTx(
	ctx context.Context,
	tx pgx.Tx,
	tripID string,
) (*FlexSettlement, error) {
	if tx == nil {
		return nil, errors.New(
			"FLEX settlement transaction is required",
		)
	}

	if _, err := uuid.Parse(tripID); err != nil {
		return nil, ErrFlexSettlementNotEligible
	}

	if err := AcquireTransactionAdvisoryLock(
		ctx,
		tx,
		"flex-settlement:"+tripID,
	); err != nil {
		return nil, fmt.Errorf(
			"lock FLEX settlement trip: %w",
			err,
		)
	}

	current, err := scanFlexSettlement(
		tx.QueryRow(ctx, `
            SELECT `+flexSettlementColumns+`
            FROM flex_settlements
            WHERE trip_id = $1
            FOR UPDATE
        `, tripID),
	)

	if err == nil {
		return current, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf(
			"read existing FLEX settlement: %w",
			err,
		)
	}

	captureKey := "flex-capture:" + tripID

	result, err := scanFlexSettlement(
		tx.QueryRow(ctx, `
            INSERT INTO flex_settlements (
                trip_id,
                authorization_id,
                amount,
                currency,
                provider,
                provider_authorization_id,
                capture_idempotency_key,
                status
            )
            SELECT
                t.id,
                fa.id,
                tf.total_amount,
                tf.currency,
                fa.provider,
                fa.provider_authorization_id,
                $2,
                'PENDING'
            FROM trips t
            JOIN trip_fares tf
                ON tf.trip_id = t.id
            JOIN ride_requests rr
                ON rr.id = t.ride_request_id
            JOIN flex_auctions a
                ON a.ride_request_id = rr.id
            JOIN flex_authorizations fa
                ON fa.auction_id = a.id
                AND fa.offer_id = a.selected_offer_id
            WHERE t.id = $1
                AND t.status = 'COMPLETED'
                AND t.deleted_at IS NULL
                AND rr.matching_mode = 'FLEX'
                AND a.status = 'CONFIRMED'
                AND fa.status = 'AUTHORIZED'
                AND fa.provider = 'STRIPE'
                AND fa.provider_authorization_id IS NOT NULL
                AND fa.amount = tf.total_amount
                AND fa.currency = tf.currency
            RETURNING `+flexSettlementColumns,
			tripID,
			captureKey,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlexSettlementNotEligible
	}
	if err != nil {
		return nil, fmt.Errorf(
			"create FLEX settlement: %w",
			err,
		)
	}

	return result, nil
}
