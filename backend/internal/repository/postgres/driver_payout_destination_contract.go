package postgres

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

var ErrDriverPayoutDestinationInvalidInput = errors.New(
	"invalid driver payout destination input",
)

type DriverPayoutDestinationBinding struct {
	DestinationID string
	Provider      string
}

func ValidateDriverPayoutDestinationBinding(
	binding DriverPayoutDestinationBinding,
) error {
	if _, err := uuid.Parse(binding.DestinationID); err != nil {
		return ErrDriverPayoutDestinationInvalidInput
	}

	provider := binding.Provider

	if provider == "" ||
		len(provider) > 50 ||
		provider != strings.TrimSpace(provider) ||
		provider != strings.ToUpper(provider) {
		return ErrDriverPayoutDestinationInvalidInput
	}

	return nil
}
