-- +goose Up

CREATE TABLE driver_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    driver_id UUID NOT NULL
        REFERENCES drivers(id),

    document_type VARCHAR(50) NOT NULL,

    file_name VARCHAR(255) NOT NULL,
    storage_key TEXT NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    file_size_bytes BIGINT NOT NULL,

    expires_at DATE,

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    verified_at TIMESTAMPTZ,
    verified_by_user_id UUID
        REFERENCES users(id),

    rejection_reason TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT chk_driver_documents_type
        CHECK (
            document_type IN (
                'TAXI_DRIVER_LICENSE',
                'DRIVING_LICENSE'
            )
        ),

    CONSTRAINT chk_driver_documents_status
        CHECK (
            status IN (
                'PENDING',
                'VERIFIED',
                'REJECTED'
            )
        ),

    CONSTRAINT chk_driver_documents_file_size
        CHECK (file_size_bytes > 0),

    CONSTRAINT chk_driver_documents_verification
        CHECK (
            (status = 'VERIFIED'
                AND verified_at IS NOT NULL
                AND verified_by_user_id IS NOT NULL
                AND rejection_reason IS NULL)
            OR
            (status = 'REJECTED'
                AND verified_at IS NULL
                AND verified_by_user_id IS NOT NULL
                AND rejection_reason IS NOT NULL)
            OR
            (status = 'PENDING'
                AND verified_at IS NULL
                AND verified_by_user_id IS NULL
                AND rejection_reason IS NULL)
        )
);

CREATE UNIQUE INDEX idx_driver_documents_active_type
ON driver_documents(driver_id, document_type)
WHERE deleted_at IS NULL;

CREATE INDEX idx_driver_documents_driver
ON driver_documents(driver_id)
WHERE deleted_at IS NULL;

CREATE INDEX idx_driver_documents_status
ON driver_documents(status)
WHERE deleted_at IS NULL;


-- +goose Down

DROP TABLE IF EXISTS driver_documents;