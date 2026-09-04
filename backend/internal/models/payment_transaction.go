package models

import (
	"encoding/json"
	"time"
)

// PaymentTransaction represents one immutable provider-facing
// financial operation associated with a logical payment.
//
// Amount is represented as a decimal string to preserve PostgreSQL
// NUMERIC precision without binary floating-point conversion.
type PaymentTransaction struct {
	BaseModel

	PaymentID string `db:"payment_id" json:"payment_id"`

	TransactionReference string `db:"transaction_reference" json:"transaction_reference"`

	Provider string `db:"provider" json:"provider"`

	ProviderTransactionID *string `db:"provider_transaction_id" json:"provider_transaction_id,omitempty"`
	IdempotencyKey        *string `db:"idempotency_key" json:"idempotency_key,omitempty"`

	TransactionType string `db:"transaction_type" json:"transaction_type"`
	Status          string `db:"status" json:"status"`

	Amount   string `db:"amount" json:"amount"`
	Currency string `db:"currency" json:"currency"`

	GatewayRequest  json.RawMessage `db:"gateway_request" json:"gateway_request,omitempty"`
	GatewayResponse json.RawMessage `db:"gateway_response" json:"gateway_response,omitempty"`

	ProcessedAt *time.Time `db:"processed_at" json:"processed_at,omitempty"`
}
