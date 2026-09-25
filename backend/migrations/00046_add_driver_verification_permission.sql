-- +goose Up

INSERT INTO permissions (name, description)
VALUES ('drivers.verify', 'Verify driver registration applications')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p
    ON p.name = 'drivers.verify'
WHERE r.name = 'SYSTEM_ADMIN'
ON CONFLICT DO NOTHING;


-- +goose Down

DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id
    FROM permissions
    WHERE name = 'drivers.verify'
)
AND role_id = (
    SELECT id
    FROM roles
    WHERE name = 'SYSTEM_ADMIN'
);

DELETE FROM permissions
WHERE name = 'drivers.verify';