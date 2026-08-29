package postgres

const farePricingProfileColumns = `
	id,
	company_id,
	branch_id,
	service_category_id,
	version,
	currency,
	base_fare,
	distance_rate_per_km,
	time_rate_per_minute,
	waiting_rate_per_minute,
	booking_fee,
	surge_multiplier,
	effective_from,
	effective_to,
	is_active,
	created_by_user_id,
	created_at
`
