-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_flex_authorization_lifecycle()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    -- Provider identity may be assigned once but never replaced or cleared.
    IF OLD.provider_authorization_id IS NOT NULL
       AND NEW.provider_authorization_id
           IS DISTINCT FROM OLD.provider_authorization_id THEN
        RAISE EXCEPTION
            'FLEX authorization provider identity cannot change'
            USING ERRCODE = '23514';
    END IF;

    -- A provider identity must never be an empty string.
    IF NEW.provider_authorization_id IS NOT NULL
       AND BTRIM(NEW.provider_authorization_id) = '' THEN
        RAISE EXCEPTION
            'FLEX authorization provider identity cannot be empty'
            USING ERRCODE = '23514';
    END IF;

    -- Successful authorization requires provider identity.
    IF NEW.status = 'AUTHORIZED'
       AND NEW.provider_authorization_id IS NULL THEN
        RAISE EXCEPTION
            'AUTHORIZED requires provider authorization identity'
            USING ERRCODE = '23514';
    END IF;

    -- Authorization timestamp must be assigned only on success.
    IF NEW.status <> 'AUTHORIZED'
       AND NEW.authorized_at IS NOT NULL THEN
        RAISE EXCEPTION
            'authorized_at requires AUTHORIZED status'
            USING ERRCODE = '23514';
    END IF;

    -- Once established, the authorization timestamp is immutable.
    IF OLD.authorized_at IS NOT NULL
       AND NEW.authorized_at IS DISTINCT FROM OLD.authorized_at THEN
        RAISE EXCEPTION
            'FLEX authorization timestamp cannot change'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.status = OLD.status THEN
        RETURN NEW;
    END IF;

    IF OLD.status = 'PENDING' THEN
        IF NEW.status = 'PROCESSING' THEN
            IF clock_timestamp() >= OLD.expires_at THEN
                RAISE EXCEPTION
                    'FLEX authorization initiation deadline expired'
                    USING ERRCODE = '23514';
            END IF;

            RETURN NEW;
        END IF;

        IF NEW.status IN ('FAILED', 'CANCELLED', 'EXPIRED') THEN
            RETURN NEW;
        END IF;
    END IF;

    IF OLD.status = 'PROCESSING'
       AND NEW.status IN ('AUTHORIZED', 'FAILED', 'CANCELLED') THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION
        'Invalid FLEX authorization transition: % -> %',
        OLD.status,
        NEW.status
        USING ERRCODE = '23514';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_enforce_flex_authorization_lifecycle
BEFORE UPDATE ON flex_authorizations
FOR EACH ROW
EXECUTE FUNCTION enforce_flex_authorization_lifecycle();

-- +goose Down

DROP TRIGGER IF EXISTS trg_enforce_flex_authorization_lifecycle
    ON flex_authorizations;

DROP FUNCTION IF EXISTS enforce_flex_authorization_lifecycle();
