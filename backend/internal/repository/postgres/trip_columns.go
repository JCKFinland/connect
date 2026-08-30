package postgres

const tripColumns = `
    id,
    ride_request_id,
    customer_id,
    driver_id,
    vehicle_id,
    company_id,
    branch_id,
    service_category_id,
    pricing_profile_id,
    fleet_id,
    status,

    estimated_distance_km,
    estimated_duration_minutes,
    actual_distance_km,
    actual_duration_minutes,

    estimated_distance_meters,
    estimated_duration_seconds,
    actual_distance_meters,
    actual_duration_seconds,

    assigned_at,
    scheduled_at,

    driver_arrived_at,
    passenger_on_board_at,
    pickup_at,
    started_at,
    completed_at,
    cancelled_at,

    cancellation_reason,
    cancelled_by,

    pickup_address,
    pickup_latitude,
    pickup_longitude,

    dropoff_address,
    dropoff_latitude,
    dropoff_longitude,

    passenger_note,

    is_active,
    deleted_at,

    created_at,
    updated_at
`
