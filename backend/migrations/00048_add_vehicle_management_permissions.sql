-- +goose Up

INSERT INTO permissions (name, description)
VALUES
    ('vehicles.read', 'View registered vehicles'),
    ('vehicles.manage', 'Create, update, and delete registered vehicles')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN (
    'SYSTEM_ADMIN',
    'COMPANY_ADMIN',
    'FLEET_MANAGER',
    'DISPATCHER'
)
AND p.name = 'vehicles.read'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN (
    'SYSTEM_ADMIN',
    'COMPANY_ADMIN',
    'FLEET_MANAGER'
)
AND p.name = 'vehicles.manage'
ON CONFLICT DO NOTHING;


-- +goose Down

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id
    FROM permissions
    WHERE name IN (
        'vehicles.read',
        'vehicles.manage'
    )
);

DELETE FROM permissions
WHERE name IN (
    'vehicles.read',
    'vehicles.manage'
);