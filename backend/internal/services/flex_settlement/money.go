package flex_settlement

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// postgresFlexAmountToCents converts a PostgreSQL monetary
// decimal string to integer cents without floating-point arithmetic.
//
// Accepted format: positive digits, decimal point, two digits.
// Example: "23.45" -> 2345.
func postgresFlexAmountToCents(amount string) (int64, error) {
	if amount == "" || strings.TrimSpace(amount) != amount {
		return 0, errors.New("invalid FLEX monetary amount")
	}

	parts := strings.Split(amount, ".")
	if len(parts) != 2 ||
		len(parts[0]) == 0 ||
		len(parts[1]) != 2 {
		return 0, fmt.Errorf(
			"invalid FLEX monetary precision: %q",
			amount,
		)
	}

	for _, part := range parts {
		for _, char := range part {
			if char < '0' || char > '9' {
				return 0, fmt.Errorf(
					"invalid FLEX monetary digits: %q",
					amount,
				)
			}
		}
	}

	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"parse FLEX monetary whole amount: %w",
			err,
		)
	}

	fraction, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"parse FLEX monetary fractional amount: %w",
			err,
		)
	}

	if whole > (math.MaxInt64-fraction)/100 {
		return 0, errors.New("FLEX monetary amount overflow")
	}

	cents := whole*100 + fraction
	if cents <= 0 {
		return 0, errors.New(
			"FLEX monetary amount must be positive",
		)
	}

	return cents, nil
}
