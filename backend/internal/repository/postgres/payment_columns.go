package postgres

const paymentColumns = `
	id,
	trip_id,
	fare_id,
	customer_id,
	status,
	payment_method,
	amount::text,
	currency,
	paid_at,
	created_at,
	updated_at
`
