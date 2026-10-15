package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SelectPendingOffer reserves an eligible driver offer.
//
// The caller must hold the authoritative ride-request and auction locks
// and execute this operation within the same PostgreSQL transaction.
func SelectPendingOffer(
	ctx context.Context,
	tx pgx.Tx,
	auctionID string,
	offerID string,
) error {
	if tx == nil {
		return errors.New("FLEX selection transaction is required")
	}
	if auctionID == "" || offerID == "" {
		return errors.New("FLEX auction and offer IDs are required")
	}

	tag, err := tx.Exec(ctx, `
		UPDATE flex_offers
		SET status = 'SELECTED',
		    responded_at = clock_timestamp(),
		    updated_at = clock_timestamp()
		WHERE id = $1
		  AND auction_id = $2
		  AND status = 'PENDING'
		  AND expires_at > clock_timestamp()
	`, offerID, auctionID)
	if err != nil {
		return fmt.Errorf("select FLEX offer: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrFlexSelectionConflict
	}

	return nil
}

// ReserveSelectedOffer atomically records the passenger's selected offer
// and transitions the auction into payment authorization.
//
// The caller must execute this operation in the same transaction as
// SelectPendingOffer. No payment is captured or authorized here.
func ReserveSelectedOffer(
	ctx context.Context,
	tx pgx.Tx,
	auctionID string,
	offerID string,
) error {
	if tx == nil {
		return errors.New("FLEX selection transaction is required")
	}
	if auctionID == "" || offerID == "" {
		return errors.New("FLEX auction and offer IDs are required")
	}

	tag, err := tx.Exec(ctx, `
		UPDATE flex_auctions
		SET status = 'PENDING_AUTHORIZATION',
		    selected_offer_id = $2,
		    resolved_at = NULL,
		    updated_at = clock_timestamp()
		WHERE id = $1
		  AND status = 'OPEN'
		  AND selected_offer_id IS NULL
		  AND expires_at > clock_timestamp()
		  AND EXISTS (
		      SELECT 1
		      FROM flex_offers
		      WHERE id = $2
		        AND auction_id = $1
		        AND status = 'SELECTED'
		  )
	`, auctionID, offerID)
	if err != nil {
		return fmt.Errorf("reserve FLEX auction offer: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrFlexSelectionConflict
	}

	return nil
}

// ErrFlexSelectionConflict indicates that the requested selection can
// no longer be completed against the current persistence state.
var ErrFlexSelectionConflict = errors.New(
	"FLEX offer selection is no longer eligible",
)
