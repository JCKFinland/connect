package payouts

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidRequest = errors.New("invalid payout provider request")
	ErrUnknownOutcome = errors.New("payout provider outcome unknown")
)

type TransferStatus string

const (
	TransferSucceeded TransferStatus = "SUCCEEDED"
	TransferFailed    TransferStatus = "FAILED"
	TransferPending   TransferStatus = "PENDING"
	TransferUnknown   TransferStatus = "UNKNOWN"
)

type TransferRequest struct {
	PayoutID       string
	IdempotencyKey string
	DestinationID  string
	AmountCents    int64
	Currency       string
}

type TransferResult struct {
	Status            TransferStatus
	ProviderReference string
}

type TransferLookup struct {
	IdempotencyKey string
}

type Provider interface {
	InitiateTransfer(
		context.Context,
		TransferRequest,
	) (TransferResult, error)

	LookupTransfer(
		context.Context,
		TransferLookup,
	) (TransferResult, error)
}

func ValidateTransferRequest(req TransferRequest) error {
	if strings.TrimSpace(req.PayoutID) == "" ||
		strings.TrimSpace(req.IdempotencyKey) == "" ||
		strings.TrimSpace(req.DestinationID) == "" ||
		req.AmountCents <= 0 ||
		req.Currency != "EUR" {
		return ErrInvalidRequest
	}

	if req.PayoutID != strings.TrimSpace(req.PayoutID) ||
		req.IdempotencyKey != strings.TrimSpace(req.IdempotencyKey) ||
		req.DestinationID != strings.TrimSpace(req.DestinationID) ||
		len(req.IdempotencyKey) > 255 {
		return ErrInvalidRequest
	}

	return nil
}

func ValidateTransferLookup(req TransferLookup) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" ||
		req.IdempotencyKey != strings.TrimSpace(req.IdempotencyKey) ||
		len(req.IdempotencyKey) > 255 {
		return ErrInvalidRequest
	}

	return nil
}

func ValidateTransferResult(result TransferResult) error {
	switch result.Status {
	case TransferSucceeded:
		if strings.TrimSpace(result.ProviderReference) == "" {
			return ErrInvalidRequest
		}
	case TransferFailed, TransferPending, TransferUnknown:
		// A provider reference may not yet exist.
	default:
		return ErrInvalidRequest
	}

	if result.ProviderReference !=
		strings.TrimSpace(result.ProviderReference) ||
		len(result.ProviderReference) > 255 {
		return ErrInvalidRequest
	}

	return nil
}
