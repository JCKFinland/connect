-- +goose Up

INSERT INTO permissions (
    name,
    description
)
VALUES (
    'rides.dispatch',
    'Dispatch ride requests and assign available drivers'
)
ON CONFLICT (name) DO NOTHING;

-- SYSTEM_ADMIN
INSERT INTO role_permissions (
    role_id,
    permission_id
)
SELECT
    r.id,
    p.id
FROM roles r
JOIN permissions p
    ON p.name = 'rides.dispatch'
WHERE r.name = 'SYSTEM_ADMIN'
ON CONFLICT DO NOTHING;

-- COMPANY_ADMIN
INSERT INTO role_permissions (
    role_id,
    permission_id
)
SELECT
    r.id,
    p.id
FROM roles r
JOIN permissions p
    ON p.name = 'rides.dispatch'
WHERE r.name = 'COMPANY_ADMIN'
ON CONFLICT DO NOTHING;

-- DISPATCHER
INSERT INTO role_permissions (
    role_id,
    permission_id
)
SELECT
    r.id,
    p.id
FROM roles r
JOIN permissions p
    ON p.name = 'rides.dispatch'
WHERE r.name = 'DISPATCHER'
ON CONFLICT DO NOTHING;


-- +goose Down

DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id
    FROM permissions
    WHERE name = 'rides.dispatch'
);

DELETE FROM permissions
WHERE name = 'rides.dispatch';