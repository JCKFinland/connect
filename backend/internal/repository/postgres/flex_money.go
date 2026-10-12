package postgres

import (
	"fmt"
	"strconv"
	"strings"
)

const flexMaxAmountCents int64 = 9_999_999_999

// flexCentsToDecimal converts positive integer cents into an exact
// NUMERIC(10,2)-compatible decimal representation.
func flexCentsToDecimal(cents int64) (string, error) {
	if cents <= 0 || cents > flexMaxAmountCents {
		return "", fmt.Errorf(
			"flex amount cents out of range: %d",
			cents,
		)
	}

	return fmt.Sprintf(
		"%d.%02d",
		cents/100,
		cents%100,
	), nil
}

// flexDecimalToCents parses a PostgreSQL monetary decimal exactly.
// No floating-point operations are used.
func flexDecimalToCents(value string) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts) == 0 {
		return 0, fmt.Errorf("invalid flex decimal amount: %q", value)
	}

	whole := parts[0]
	if whole == "" || strings.HasPrefix(whole, "-") ||
		strings.HasPrefix(whole, "+") {
		return 0, fmt.Errorf("invalid flex decimal amount: %q", value)
	}

	for _, ch := range whole {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid flex decimal amount: %q", value)
		}
	}

	wholeAmount, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || wholeAmount > flexMaxAmountCents/100 {
		return 0, fmt.Errorf("flex decimal amount out of range: %q", value)
	}

	var fraction int64

	if len(parts) == 2 {
		decimalPlaces := parts[1]

		if len(decimalPlaces) == 0 || len(decimalPlaces) > 2 {
			return 0, fmt.Errorf(
				"invalid flex decimal precision: %q",
				value,
			)
		}

		for _, ch := range decimalPlaces {
			if ch < '0' || ch > '9' {
				return 0, fmt.Errorf(
					"invalid flex decimal amount: %q",
					value,
				)
			}
		}

		if len(decimalPlaces) == 1 {
			decimalPlaces += "0"
		}

		fraction, err = strconv.ParseInt(decimalPlaces, 10, 64)
		if err != nil {
			return 0, fmt.Errorf(
				"invalid flex decimal amount: %q",
				value,
			)
		}
	}

	cents := wholeAmount*100 + fraction

	if cents <= 0 || cents > flexMaxAmountCents {
		return 0, fmt.Errorf(
			"flex decimal amount out of range: %q",
			value,
		)
	}

	return cents, nil
}
