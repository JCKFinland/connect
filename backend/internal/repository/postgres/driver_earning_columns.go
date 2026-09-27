package postgres

const driverEarningColumns = `
	id,
	trip_id,
	driver_id,
	company_id,
	fare_id,
	payment_id,
	gross_amount::text,
	commission_amount::text,
	bonus_amount::text,
	tip_amount::text,
	adjustment_amount::text,
	tax_withheld::text,
	net_amount::text,
	currency,
	settlement_status,
	calculated_at,
	settled_at,
	created_at,
	updated_at
`
