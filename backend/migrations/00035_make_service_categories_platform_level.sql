-- +goose Up

-- ============================================================
-- MAKE SERVICE CATEGORIES PLATFORM-LEVEL
--
-- Ride requests are platform-level and may be fulfilled by
-- different companies during dispatch.
--
-- Therefore a service category must represent the product the
-- customer requests across CONNECT, while company-specific fare
-- differences belong in fare_pricing_profiles.
-- ============================================================

-- Drop indexes that depend on service_categories.company_id.
DROP INDEX IF EXISTS uq_service_categories_company_code;
DROP INDEX IF EXISTS idx_service_categories_company;
DROP INDEX IF EXISTS idx_service_categories_active;

-- Drop the foreign key before removing the column.
ALTER TABLE service_categories
    DROP CONSTRAINT IF EXISTS fk_service_categories_company;

-- Remove company ownership from the category.
ALTER TABLE service_categories
    DROP COLUMN company_id;

-- Category codes are now unique across the CONNECT platform.
CREATE UNIQUE INDEX uq_service_categories_code
ON service_categories (UPPER(code));

-- Efficient lookup of active platform categories.
CREATE INDEX idx_service_categories_active
ON service_categories (is_active);


-- +goose Down

-- Remove platform-level indexes.
DROP INDEX IF EXISTS uq_service_categories_code;
DROP INDEX IF EXISTS idx_service_categories_active;

-- Restore company ownership.
ALTER TABLE service_categories
    ADD COLUMN company_id UUID;

-- IMPORTANT:
-- Existing platform-level rows cannot be safely assigned to a
-- company automatically, so company_id remains nullable during
-- rollback restoration. The original NOT NULL condition can only
-- be restored after valid company ownership has been supplied.

ALTER TABLE service_categories
    ADD CONSTRAINT fk_service_categories_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT;

CREATE UNIQUE INDEX uq_service_categories_company_code
ON service_categories (
    company_id,
    UPPER(code)
);

CREATE INDEX idx_service_categories_company
ON service_categories(company_id);

CREATE INDEX idx_service_categories_active
ON service_categories(company_id, is_active);