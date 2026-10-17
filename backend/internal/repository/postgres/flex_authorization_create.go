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
	ErrFlexAuthorizationAccessDenied = errors.New(
		"FLEX authorization access denied",
	)
	ErrFlexAuthorizationNotEligible = errors.New(
		"FLEX authorization is not eligible",
	)
)

type FlexAuthorization struct {
	ID                      string
	AuctionID               string
	OfferID                 string
	PassengerID             string
	AmountCents             int64
	Currency                string
	Provider                string
	IdempotencyKey          string
	Status                  string
	ProviderAuthorizationID *string
}

const flexAuthorizationColumns = `
	id,
	auction_id,
	offer_id,
	passenger_id,
	amount::text,
	currency,
	provider,
	idempotency_key,
	status,
	provider_authorization_id
`

func scanFlexAuthorization(row pgx.Row) (*FlexAuthorization, error) {
	a := &FlexAuthorization{}
	var amount string

	err := row.Scan(
		&a.ID,
		&a.AuctionID,
		&a.OfferID,
		&a.PassengerID,
		&amount,
		&a.Currency,
		&a.Provider,
		&a.IdempotencyKey,
		&a.Status,
		&a.ProviderAuthorizationID,
	)
	if err != nil {
		return nil, err
	}

	a.AmountCents, err = flexDecimalToCents(amount)
	if err != nil {
		return nil, fmt.Errorf("decode FLEX authorization amount: %w", err)
	}

	return a, nil
}

// CreateFlexAuthorization establishes the immutable local authorization
// identity for a passenger-selected FLEX offer.
//
// No payment provider is contacted by this operation.
func CreateFlexAuthorization(
	ctx context.Context,
	db *pgxpool.Pool,
	rideRequestID string,
	auctionID string,
	passengerID string,
) (*FlexAuthorization, error) {
	if db == nil {
		return nil, errors.New("FLEX authorization database is required")
	}

	if _, err := uuid.Parse(rideRequestID); err != nil {
		return nil, ErrFlexAuthorizationAccessDenied
	}
	if _, err := uuid.Parse(auctionID); err != nil {
		return nil, ErrFlexAuthorizationAccessDenied
	}
	if _, err := uuid.Parse(passengerID); err != nil {
		return nil, ErrFlexAuthorizationAccessDenied
	}

	// Ownership must be checked before exposing auction information.
	var ownerID string
	err := db.QueryRow(ctx, `
		SELECT customer_id
		FROM ride_requests
		WHERE id = $1
	`, rideRequestID).Scan(&ownerID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlexAuthorizationAccessDenied
	}
	if err != nil {
		return nil, fmt.Errorf("resolve FLEX passenger ownership: %w", err)
	}
	if ownerID != passengerID {
		return nil, ErrFlexAuthorizationAccessDenied
	}

	var result *FlexAuthorization

	err = RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		if err := AcquireTransactionAdvisoryLock(
			ctx,
			tx,
			"ride-dispatch:"+rideRequestID,
		); err != nil {
			return fmt.Errorf("lock FLEX authorization ride: %w", err)
		}

		var lockedOwner string
		var matchingMode string
		var rideStatus string
		var rideNotExpired bool

		err := tx.QueryRow(ctx, `
			SELECT
				customer_id,
				matching_mode,
				status,
				(expires_at IS NULL OR expires_at > clock_timestamp())
			FROM ride_requests
			WHERE id = $1
			FOR UPDATE
		`, rideRequestID).Scan(
			&lockedOwner,
			&matchingMode,
			&rideStatus,
			&rideNotExpired,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexAuthorizationAccessDenied
		}
		if err != nil {
			return fmt.Errorf("lock FLEX authorization ride: %w", err)
		}

		if lockedOwner != passengerID {
			return ErrFlexAuthorizationAccessDenied
		}
		if matchingMode != "FLEX" ||
			rideStatus != "PENDING" ||
			!rideNotExpired {
			return ErrFlexAuthorizationNotEligible
		}

		var selectedOfferID *string
		var auctionStatus string
		var currency string

		err = tx.QueryRow(ctx, `
			SELECT
				selected_offer_id,
				status,
				currency
			FROM flex_auctions
			WHERE id = $1
			  AND ride_request_id = $2
			FOR UPDATE
		`, auctionID, rideRequestID).Scan(
			&selectedOfferID,
			&auctionStatus,
			&currency,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexAuthorizationNotEligible
		}
		if err != nil {
			return fmt.Errorf("lock FLEX authorization auction: %w", err)
		}

		if auctionStatus != "PENDING_AUTHORIZATION" ||
			selectedOfferID == nil ||
			currency != "EUR" {
			return ErrFlexAuthorizationNotEligible
		}

		var offeredAmount string
		var offerStatus string

		err = tx.QueryRow(ctx, `
			SELECT offered_amount::text, status
			FROM flex_offers
			WHERE id = $1
			  AND auction_id = $2
			FOR UPDATE
		`, *selectedOfferID, auctionID).Scan(
			&offeredAmount,
			&offerStatus,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexAuthorizationNotEligible
		}
		if err != nil {
			return fmt.Errorf("lock FLEX authorization offer: %w", err)
		}
		if offerStatus != "SELECTED" {
			return ErrFlexAuthorizationNotEligible
		}

		amountCents, err := flexDecimalToCents(offeredAmount)
		if err != nil {
			return fmt.Errorf("invalid selected FLEX fare: %w", err)
		}
		if amountCents <= 0 {
			return ErrFlexAuthorizationNotEligible
		}

		// The selected offer is immutable. This identity must remain
		// stable across HTTP retries and later provider retries.
		idempotencyKey := "flex-authorize:" + auctionID

		result, err = scanFlexAuthorization(tx.QueryRow(ctx, `
			INSERT INTO flex_authorizations (
				auction_id,
				offer_id,
				passenger_id,
				amount,
				currency,
				provider,
				idempotency_key,
				status,
				expires_at
			)
			VALUES (
				$1, $2, $3, $4::numeric, $5,
				'STRIPE', $6, 'PENDING',
				clock_timestamp() + INTERVAL '5 minutes'
			)
			ON CONFLICT (auction_id)
			DO UPDATE SET
				updated_at = flex_authorizations.updated_at
			WHERE flex_authorizations.offer_id = EXCLUDED.offer_id
			  AND flex_authorizations.passenger_id = EXCLUDED.passenger_id
			  AND flex_authorizations.amount = EXCLUDED.amount
			  AND flex_authorizations.currency = EXCLUDED.currency
			  AND flex_authorizations.idempotency_key = EXCLUDED.idempotency_key
			RETURNING `+flexAuthorizationColumns,
			auctionID,
			*selectedOfferID,
			passengerID,
			offeredAmount,
			currency,
			idempotencyKey,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexAuthorizationNotEligible
		}
		if err != nil {
			return fmt.Errorf("create FLEX authorization: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
