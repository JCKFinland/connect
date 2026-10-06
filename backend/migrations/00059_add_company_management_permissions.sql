-- +goose Up

INSERT INTO permissions (name, description)
VALUES
    ('companies.read', 'View companies'),
    ('companies.manage', 'Update, archive, deactivate, and reactivate companies')
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
AND p.name = 'companies.read'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name IN (
    'SYSTEM_ADMIN',
    'COMPANY_ADMIN'
)
AND p.name = 'companies.manage'
ON CONFLICT DO NOTHING;

-- +goose Down

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id
    FROM permissions
    WHERE name IN (
        'companies.read',
        'companies.manage'
    )
);

DELETE FROM permissions
WHERE name IN (
    'companies.read',
    'companies.manage'
);
