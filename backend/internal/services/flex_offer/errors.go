package flex_offer

import "errors"

var (
	ErrInvalidCounterOfferAmount = errors.New(
		"invalid FLEX counter-offer amount",
	)

	ErrAccessDenied = errors.New(
		"FLEX driver offer access denied",
	)

	ErrAuctionNotEligible = errors.New(
		"FLEX auction is not eligible for driver offers",
	)

	ErrDriverNotEligible = errors.New(
		"driver is not eligible to submit FLEX offers",
	)

	ErrOfferAlreadyExists = errors.New(
		"driver has already submitted an offer for this FLEX auction",
	)
)
