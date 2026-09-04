package postgres

const paymentTransactionColumns = `
	id,
	payment_id,
	transaction_reference,
	provider,
	provider_transaction_id,
	idempotency_key,
	transaction_type,
	status,
	amount::text,
	currency,
	gateway_request,
	gateway_response,
	processed_at,
	created_at,
	updated_at
`
