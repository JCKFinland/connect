-- +goose Up

-- FLEX settlement health exposes platform-wide financial
-- operational information and must be restricted to SYSTEM_ADMIN.

INSERT INTO permissions (
    name,
    description
)
VALUES (
    'flex_settlements.health.read',
    'View platform-wide FLEX settlement operational health'
)
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (
    role_id,
    permission_id
)
SELECT
    r.id,
    p.id
FROM roles r
JOIN permissions p
    ON p.name = 'flex_settlements.health.read'
WHERE r.name = 'SYSTEM_ADMIN'
ON CONFLICT DO NOTHING;


-- +goose Down

DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id
    FROM permissions
    WHERE name = 'flex_settlements.health.read'
)
AND role_id = (
    SELECT id
    FROM roles
    WHERE name = 'SYSTEM_ADMIN'
);

DELETE FROM permissions
WHERE name = 'flex_settlements.health.read';
