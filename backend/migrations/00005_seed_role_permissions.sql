-- +goose Up

-------------------------------------------------------
-- SYSTEM_ADMIN
-------------------------------------------------------

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'SYSTEM_ADMIN'
ON CONFLICT DO NOTHING;

-------------------------------------------------------
-- COMPANY_ADMIN
-------------------------------------------------------

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p
ON p.name IN (

    'users.read',
    'users.create',
    'users.update',

    'drivers.read',
    'drivers.manage',

    'fleets.read',
    'fleets.manage',

    'rides.read',
    'rides.update',
    'rides.cancel',

    'payments.read',
    'payments.manage',

    'profile.read',
    'profile.update'
)
WHERE r.name = 'COMPANY_ADMIN'
ON CONFLICT DO NOTHING;

-------------------------------------------------------
-- FLEET_MANAGER
-------------------------------------------------------

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p
ON p.name IN (

    'drivers.read',
    'drivers.manage',

    'fleets.read',
    'fleets.manage',

    'rides.read',
    'rides.update',

    'profile.read',
    'profile.update'
)
WHERE r.name = 'FLEET_MANAGER'
ON CONFLICT DO NOTHING;

-------------------------------------------------------
-- DISPATCHER
-------------------------------------------------------

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p
ON p.name IN (

    'rides.read',
    'rides.update',
    'rides.cancel',

    'drivers.read',

    'profile.read',
    'profile.update'
)
WHERE r.name = 'DISPATCHER'
ON CONFLICT DO NOTHING;

-------------------------------------------------------
-- DRIVER
-------------------------------------------------------

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p
ON p.name IN (

    'rides.read',

    'profile.read',
    'profile.update'
)
WHERE r.name = 'DRIVER'
ON CONFLICT DO NOTHING;

-------------------------------------------------------
-- CUSTOMER
-------------------------------------------------------

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p
ON p.name IN (

    'rides.create',

    'profile.read',
    'profile.update'
)
WHERE r.name = 'CUSTOMER'
ON CONFLICT DO NOTHING;

-- +goose Down

DELETE FROM role_permissions;