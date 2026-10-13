package flex_pricing

import (
	"errors"
	"math"
)

const (
	maxAmountCents  int64 = 9_999_999_999
	bidRangePercent int64 = 10
)

var (
	ErrInvalidBaseline = errors.New("invalid FLEX fare baseline")
	ErrInvalidOffer    = errors.New("invalid FLEX passenger offer")
	ErrOfferOutOfRange = errors.New("FLEX passenger offer outside permitted range")
)

type BidBounds struct {
	BaselineCents int64
	MinimumCents  int64
	MaximumCents  int64
}

func BoundsFromEstimate(totalEUR float64, currency string) (BidBounds, error) {
	if currency != "EUR" ||
		math.IsNaN(totalEUR) ||
		math.IsInf(totalEUR, 0) ||
		totalEUR <= 0 ||
		totalEUR > float64(maxAmountCents)/100 {
		return BidBounds{}, ErrInvalidBaseline
	}

	baseline := int64(math.Round(totalEUR * 100))
	if baseline <= 0 || baseline > maxAmountCents {
		return BidBounds{}, ErrInvalidBaseline
	}

	// Integer arithmetic prevents floating-point drift in bidding limits.
	minimum := baseline * (100 - bidRangePercent) / 100

	// Calculate the upward adjustment without overflowing int64.
	// The increment is ceil(baseline * bidRangePercent / 100).
	increment := (baseline/100)*bidRangePercent +
		((baseline%100)*bidRangePercent+99)/100

	maximum := maxAmountCents
	if increment <= maxAmountCents-baseline {
		maximum = baseline + increment
	}

	if minimum < 1 {
		minimum = 1
	}

	return BidBounds{
		BaselineCents: baseline,
		MinimumCents:  minimum,
		MaximumCents:  maximum,
	}, nil
}

func ValidateOffer(
	offerCents int64,
	bounds BidBounds,
) error {
	if offerCents <= 0 || offerCents > maxAmountCents {
		return ErrInvalidOffer
	}

	if bounds.BaselineCents <= 0 ||
		bounds.MinimumCents <= 0 ||
		bounds.MaximumCents < bounds.MinimumCents ||
		bounds.MaximumCents > maxAmountCents {
		return ErrInvalidBaseline
	}

	if offerCents < bounds.MinimumCents ||
		offerCents > bounds.MaximumCents {
		return ErrOfferOutOfRange
	}

	return nil
}
