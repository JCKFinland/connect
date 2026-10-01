-- +goose Up

ALTER TABLE driver_documents
    ADD COLUMN revoked_at TIMESTAMPTZ,
    ADD COLUMN revoked_by_user_id UUID
        REFERENCES users(id),
    ADD COLUMN revocation_reason TEXT;

ALTER TABLE driver_documents
    DROP CONSTRAINT chk_driver_documents_status,
    DROP CONSTRAINT chk_driver_documents_verification;

ALTER TABLE driver_documents
    ADD CONSTRAINT chk_driver_documents_status
        CHECK (
            status IN (
                'PENDING',
                'VERIFIED',
                'REJECTED',
                'REVOKED'
            )
        ),
    ADD CONSTRAINT chk_driver_documents_verification
        CHECK (
            (
                status = 'VERIFIED'
                AND verified_at IS NOT NULL
                AND verified_by_user_id IS NOT NULL
                AND rejection_reason IS NULL
                AND revoked_at IS NULL
                AND revoked_by_user_id IS NULL
                AND revocation_reason IS NULL
            )
            OR
            (
                status = 'REJECTED'
                AND verified_at IS NULL
                AND verified_by_user_id IS NOT NULL
                AND rejection_reason IS NOT NULL
                AND revoked_at IS NULL
                AND revoked_by_user_id IS NULL
                AND revocation_reason IS NULL
            )
            OR
            (
                status = 'PENDING'
                AND verified_at IS NULL
                AND verified_by_user_id IS NULL
                AND rejection_reason IS NULL
                AND revoked_at IS NULL
                AND revoked_by_user_id IS NULL
                AND revocation_reason IS NULL
            )
            OR
            (
                status = 'REVOKED'
                AND verified_at IS NOT NULL
                AND verified_by_user_id IS NOT NULL
                AND rejection_reason IS NULL
                AND revoked_at IS NOT NULL
                AND revoked_by_user_id IS NOT NULL
                AND NULLIF(BTRIM(revocation_reason), '') IS NOT NULL
            )
        );

-- +goose Down

ALTER TABLE driver_documents
    DROP CONSTRAINT chk_driver_documents_verification,
    DROP CONSTRAINT chk_driver_documents_status;

-- The pre-revocation schema cannot represent REVOKED. Preserve the security
-- invariant during rollback by retiring revoked credentials instead of
-- restoring them as active VERIFIED documents.
UPDATE driver_documents
SET
    status = 'VERIFIED',
    deleted_at = COALESCE(deleted_at, NOW()),
    updated_at = NOW()
WHERE status = 'REVOKED';

ALTER TABLE driver_documents
    DROP COLUMN revocation_reason,
    DROP COLUMN revoked_by_user_id,
    DROP COLUMN revoked_at;

ALTER TABLE driver_documents
    ADD CONSTRAINT chk_driver_documents_status
        CHECK (
            status IN (
                'PENDING',
                'VERIFIED',
                'REJECTED'
            )
        ),
    ADD CONSTRAINT chk_driver_documents_verification
        CHECK (
            (
                status = 'VERIFIED'
                AND verified_at IS NOT NULL
                AND verified_by_user_id IS NOT NULL
                AND rejection_reason IS NULL
            )
            OR
            (
                status = 'REJECTED'
                AND verified_at IS NULL
                AND verified_by_user_id IS NOT NULL
                AND rejection_reason IS NOT NULL
            )
            OR
            (
                status = 'PENDING'
                AND verified_at IS NULL
                AND verified_by_user_id IS NULL
                AND rejection_reason IS NULL
            )
        );
