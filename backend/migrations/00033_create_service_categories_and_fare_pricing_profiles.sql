-- +goose Up

-- ============================================================
-- SERVICE CATEGORIES
--
-- Commercial ride/service products offered by a company.
--
-- Examples:
--   BASIC
--   COMFORT
--   ELECTRIC
--   VAN
--   PETS
--   PACKAGE
--   VAN_PLUS
--
-- These are intentionally separate from vehicles.vehicle_type.
-- A vehicle type describes the physical vehicle; a service
-- category describes what the customer is buying.
-- ============================================================

CREATE TABLE IF NOT EXISTS service_categories
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    company_id UUID NOT NULL,

    code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_service_categories_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_service_categories_code_not_blank
        CHECK (BTRIM(code) <> ''),

    CONSTRAINT chk_service_categories_name_not_blank
        CHECK (BTRIM(name) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_service_categories_company_code
ON service_categories
(
    company_id,
    UPPER(code)
);

CREATE INDEX IF NOT EXISTS idx_service_categories_company
ON service_categories(company_id);

CREATE INDEX IF NOT EXISTS idx_service_categories_active
ON service_categories(company_id, is_active);


-- ============================================================
-- FARE PRICING PROFILES
--
-- Immutable/effective-dated pricing definitions.
--
-- A completed trip will eventually resolve one trusted profile
-- server-side and freeze its values into trip_fares.
--
-- We intentionally do NOT store Finnish VAT/tax rules here yet.
-- Tax treatment will be added after the legal/fiscal layer is
-- designed from authoritative current requirements.
-- ============================================================

CREATE TABLE IF NOT EXISTS fare_pricing_profiles
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    company_id UUID NOT NULL,
    branch_id UUID,
    service_category_id UUID NOT NULL,

    version VARCHAR(50) NOT NULL,

    currency VARCHAR(3) NOT NULL DEFAULT 'EUR',

    base_fare NUMERIC(10,2) NOT NULL DEFAULT 0,

    distance_rate_per_km NUMERIC(10,4)
        NOT NULL DEFAULT 0,

    time_rate_per_minute NUMERIC(10,4)
        NOT NULL DEFAULT 0,

    waiting_rate_per_minute NUMERIC(10,4)
        NOT NULL DEFAULT 0,

    booking_fee NUMERIC(10,2)
        NOT NULL DEFAULT 0,

    surge_multiplier NUMERIC(8,4)
        NOT NULL DEFAULT 1,

    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_by_user_id UUID,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_fare_pricing_profiles_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_fare_pricing_profiles_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_fare_pricing_profiles_category
        FOREIGN KEY (service_category_id)
        REFERENCES service_categories(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_fare_pricing_profiles_created_by
        FOREIGN KEY (created_by_user_id)
        REFERENCES users(id)
        ON DELETE SET NULL,

    CONSTRAINT chk_fare_pricing_profiles_version_not_blank
        CHECK (BTRIM(version) <> ''),

    CONSTRAINT chk_fare_pricing_profiles_currency
        CHECK (
            CHAR_LENGTH(currency) = 3
            AND currency = UPPER(currency)
        ),

    CONSTRAINT chk_fare_pricing_profiles_base_fare
        CHECK (base_fare >= 0),

    CONSTRAINT chk_fare_pricing_profiles_distance_rate
        CHECK (distance_rate_per_km >= 0),

    CONSTRAINT chk_fare_pricing_profiles_time_rate
        CHECK (time_rate_per_minute >= 0),

    CONSTRAINT chk_fare_pricing_profiles_waiting_rate
        CHECK (waiting_rate_per_minute >= 0),

    CONSTRAINT chk_fare_pricing_profiles_booking_fee
        CHECK (booking_fee >= 0),

    CONSTRAINT chk_fare_pricing_profiles_surge_multiplier
        CHECK (surge_multiplier >= 1),

    CONSTRAINT chk_fare_pricing_profiles_effective_period
        CHECK (
            effective_to IS NULL
            OR effective_to > effective_from
        )
);


-- A version identifies an immutable pricing definition for a
-- company. This gives us a stable audit/reference value.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fare_pricing_profiles_company_version
ON fare_pricing_profiles
(
    company_id,
    version
);


-- Only one open-ended active profile may exist for the same
-- company/branch/category scope.
--
-- NULLS NOT DISTINCT means company-level pricing where branch_id
-- is NULL is also protected from duplicate open-ended profiles.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fare_pricing_profiles_open_active
ON fare_pricing_profiles
(
    company_id,
    branch_id,
    service_category_id
)
NULLS NOT DISTINCT
WHERE effective_to IS NULL
  AND is_active = TRUE;


-- Resolver lookup:
--
-- company + optional branch + category + effective timestamp.
CREATE INDEX IF NOT EXISTS idx_fare_pricing_profiles_resolution
ON fare_pricing_profiles
(
    company_id,
    service_category_id,
    branch_id,
    effective_from DESC
)
WHERE is_active = TRUE;


CREATE INDEX IF NOT EXISTS idx_fare_pricing_profiles_category
ON fare_pricing_profiles(service_category_id);


-- +goose Down

DROP TABLE IF EXISTS fare_pricing_profiles;

DROP TABLE IF EXISTS service_categories;