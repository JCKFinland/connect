package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrFlexConfirmationConflict = errors.New(
	"FLEX confirmation state conflict",
)

// AcceptSelectedFlexOffer transitions exactly one selected offer to ACCEPTED.
//
// The caller must already hold the selected offer row lock and must own
// the surrounding transaction. No transaction is started or committed here.
func AcceptSelectedFlexOffer(
	ctx context.Context,
	tx pgx.Tx,
	auctionID string,
	offerID string,
) error {
	if tx == nil || auctionID == "" || offerID == "" {
		return ErrFlexConfirmationConflict
	}

	result, err := tx.Exec(ctx, `
		UPDATE flex_offers
		SET status = 'ACCEPTED',
		    responded_at = COALESCE(
		        responded_at,
		        clock_timestamp()
		    ),
		    updated_at = clock_timestamp()
		WHERE id = $1
		  AND auction_id = $2
		  AND status = 'SELECTED'
	`, offerID, auctionID)
	if err != nil {
		return fmt.Errorf("accept selected FLEX offer: %w", err)
	}

	if result.RowsAffected() != 1 {
		return ErrFlexConfirmationConflict
	}

	return nil
}

// ConfirmAuthorizedFlexAuction transitions a reserved auction to CONFIRMED.
//
// The selected offer must already be ACCEPTED. The caller must hold the
// auction and offer row locks inside the same transaction.
func ConfirmAuthorizedFlexAuction(
	ctx context.Context,
	tx pgx.Tx,
	auctionID string,
	offerID string,
) error {
	if tx == nil || auctionID == "" || offerID == "" {
		return ErrFlexConfirmationConflict
	}

	result, err := tx.Exec(ctx, `
		UPDATE flex_auctions AS a
		SET status = 'CONFIRMED',
		    resolved_at = clock_timestamp(),
		    updated_at = clock_timestamp()
		WHERE a.id = $1
		  AND a.selected_offer_id = $2
		  AND a.status = 'PENDING_AUTHORIZATION'
		  AND EXISTS (
		      SELECT 1
		      FROM flex_offers AS o
		      WHERE o.id = $2
		        AND o.auction_id = a.id
		        AND o.status = 'ACCEPTED'
		  )
		  AND EXISTS (
		      SELECT 1
		      FROM flex_authorizations AS fa
		      WHERE fa.auction_id = a.id
		        AND fa.offer_id = $2
		        AND fa.status = 'AUTHORIZED'
		        AND fa.provider = 'STRIPE'
		        AND fa.provider_authorization_id IS NOT NULL
		        AND BTRIM(fa.provider_authorization_id) <> ''
		  )
	`, auctionID, offerID)
	if err != nil {
		return fmt.Errorf("confirm authorized FLEX auction: %w", err)
	}

	if result.RowsAffected() != 1 {
		return ErrFlexConfirmationConflict
	}

	return nil
}
