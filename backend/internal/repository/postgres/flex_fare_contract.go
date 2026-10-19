package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrFlexFareContractInvalid indicates that the confirmed FLEX
// payment and pricing records do not form a valid fare contract.
var ErrFlexFareContractInvalid = errors.New(
	"invalid confirmed FLEX fare contract",
)

// FlexFareContract is the immutable, authorized price that must
// be used when completing a confirmed FLEX trip.
//
// AmountCents is authoritative. Callers must never replace it
// with a metered calculation or a client-provided amount.
type FlexFareContract struct {
	TripID           string
	AmountCents      int64
	Currency         string
	PricingProfileID string
	PricingVersion   string
}

// ResolveConfirmedFlexFareContract validates the confirmed FLEX
// auction, accepted offer, authorized Stripe payment and frozen
// pricing provenance.
//
// The caller must supply its existing transaction-bound DBTX.
// This function performs no writes and makes no Stripe requests.
//
// The caller is responsible for holding the authoritative trip
// lock and validating the trip lifecycle before invoking it.
func ResolveConfirmedFlexFareContract(
	ctx context.Context,
	db DBTX,
	tripID string,
) (*FlexFareContract, error) {
	if db == nil {
		return nil, errors.New("FLEX fare database is not configured")
	}
	if tripID == "" {
		return nil, ErrFlexFareContractInvalid
	}

	const query = `
		SELECT
			fo.offered_amount::text,
			fa.currency,
			fa.pricing_profile_id,
			fa.pricing_version
		FROM trips t
		JOIN ride_requests rr
			ON rr.id = t.ride_request_id
		JOIN flex_auctions fa
			ON fa.ride_request_id = rr.id
		JOIN flex_offers fo
			ON fo.id = fa.selected_offer_id
			AND fo.auction_id = fa.id
		JOIN flex_authorizations auth
			ON auth.auction_id = fa.id
			AND auth.offer_id = fo.id
			AND auth.passenger_id = rr.customer_id
		JOIN fare_pricing_profiles fp
			ON fp.id = fa.pricing_profile_id
		WHERE t.id = $1
		  AND rr.matching_mode = 'FLEX'
		  AND t.customer_id = rr.customer_id
		  AND fa.status = 'CONFIRMED'
		  AND fo.status = 'ACCEPTED'
		  AND auth.status = 'AUTHORIZED'
		  AND auth.provider = 'STRIPE'
		  AND auth.provider_authorization_id IS NOT NULL
		  AND length(btrim(auth.provider_authorization_id)) > 0
		  AND auth.currency = 'EUR'
		  AND fa.currency = 'EUR'
		  AND auth.amount = fo.offered_amount
		  AND fo.offered_amount > 0
		  AND t.driver_id = fo.driver_id
		  AND t.vehicle_id = fo.vehicle_id
		  AND t.company_id = fo.company_id
		  AND t.branch_id = fo.branch_id
		  AND t.fleet_id = fo.fleet_id
		  AND t.pricing_profile_id = fa.pricing_profile_id
		  AND fp.version = fa.pricing_version
		  AND fp.company_id = t.company_id
		  AND fp.currency = fa.currency
		  AND (
			t.service_category_id IS NULL
			OR t.service_category_id = fp.service_category_id
		  )
	`

	var (
		amountText string
		contract   FlexFareContract
	)

	err := db.QueryRow(ctx, query, tripID).Scan(
		&amountText,
		&contract.Currency,
		&contract.PricingProfileID,
		&contract.PricingVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlexFareContractInvalid
	}
	if err != nil {
		return nil, fmt.Errorf(
			"resolve confirmed FLEX fare contract: %w",
			err,
		)
	}

	// Reuse CONNECT's strict decimal-to-cents conversion.
	// Never round or use floating-point arithmetic for authorization.
	amountCents, err := flexDecimalToCents(amountText)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid authorized amount: %v",
			ErrFlexFareContractInvalid,
			err,
		)
	}

	contract.TripID = tripID
	contract.AmountCents = amountCents

	if contract.AmountCents <= 0 ||
		contract.AmountCents > 9_999_999_999 ||
		contract.Currency != "EUR" ||
		contract.PricingProfileID == "" ||
		contract.PricingVersion == "" {
		return nil, ErrFlexFareContractInvalid
	}

	return &contract, nil
}
