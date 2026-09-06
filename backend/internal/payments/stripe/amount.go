package stripe

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidAmount = errors.New(
	"invalid Stripe amount",
)

func amountToMinorUnits(
	amount string,
	currency string,
) (int64, error) {
	amount = strings.TrimSpace(amount)
	currency = strings.ToUpper(
		strings.TrimSpace(currency),
	)

	if amount == "" {
		return 0, fmt.Errorf(
			"%w: amount is required",
			ErrInvalidAmount,
		)
	}

	if currency == "" {
		return 0, fmt.Errorf(
			"%w: currency is required",
			ErrInvalidAmount,
		)
	}

	// CONNECT currently executes Stripe payments in EUR.
	// EUR uses two decimal minor units.
	if currency != "EUR" {
		return 0, fmt.Errorf(
			"%w: unsupported currency %s",
			ErrInvalidAmount,
			currency,
		)
	}

	parts := strings.Split(amount, ".")

	if len(parts) > 2 {
		return 0, fmt.Errorf(
			"%w: malformed amount %s",
			ErrInvalidAmount,
			amount,
		)
	}

	whole := parts[0]

	if whole == "" {
		whole = "0"
	}

	fraction := ""

	if len(parts) == 2 {
		fraction = parts[1]
	}

	if len(fraction) > 2 {
		return 0, fmt.Errorf(
			"%w: too many decimal places in %s",
			ErrInvalidAmount,
			amount,
		)
	}

	for len(fraction) < 2 {
		fraction += "0"
	}

	wholeUnits, err :=
		strconv.ParseInt(
			whole,
			10,
			64,
		)
	if err != nil {
		return 0, fmt.Errorf(
			"%w: invalid whole amount %s",
			ErrInvalidAmount,
			amount,
		)
	}

	fractionUnits, err :=
		strconv.ParseInt(
			fraction,
			10,
			64,
		)
	if err != nil {
		return 0, fmt.Errorf(
			"%w: invalid fractional amount %s",
			ErrInvalidAmount,
			amount,
		)
	}

	if wholeUnits < 0 ||
		fractionUnits < 0 {

		return 0, fmt.Errorf(
			"%w: amount must be positive",
			ErrInvalidAmount,
		)
	}

	minorUnits :=
		wholeUnits*100 +
			fractionUnits

	if minorUnits <= 0 {
		return 0, fmt.Errorf(
			"%w: amount must be greater than zero",
			ErrInvalidAmount,
		)
	}

	return minorUnits, nil
}
