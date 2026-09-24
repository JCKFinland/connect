-- +goose Up

INSERT INTO permissions (name, description)
VALUES ('driver.operations', 'Access driver operational functions')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p
    ON p.name = 'driver.operations'
WHERE r.name = 'DRIVER'
ON CONFLICT DO NOTHING;


-- +goose Down

DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id
    FROM permissions
    WHERE name = 'driver.operations'
)
AND role_id = (
    SELECT id
    FROM roles
    WHERE name = 'DRIVER'
);

DELETE FROM permissions
WHERE name = 'driver.operations';