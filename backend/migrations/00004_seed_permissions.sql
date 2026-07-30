-- +goose Up

INSERT INTO permissions (name, description)
VALUES
    ('users.read',        'View users'),
    ('users.create',      'Create users'),
    ('users.update',      'Update users'),
    ('users.delete',      'Delete users'),

    ('roles.read',        'View roles'),
    ('roles.manage',      'Manage roles'),

    ('drivers.read',      'View drivers'),
    ('drivers.manage',    'Manage drivers'),

    ('fleets.read',       'View fleets'),
    ('fleets.manage',     'Manage fleets'),

    ('rides.create',      'Create rides'),
    ('rides.read',        'View rides'),
    ('rides.update',      'Update rides'),
    ('rides.cancel',      'Cancel rides'),

    ('payments.read',     'View payments'),
    ('payments.manage',   'Manage payments'),

    ('profile.read',      'View own profile'),
    ('profile.update',    'Update own profile')

ON CONFLICT (name) DO NOTHING;

-- +goose Down

DELETE FROM permissions
WHERE name IN (
    'users.read',
    'users.create',
    'users.update',
    'users.delete',

    'roles.read',
    'roles.manage',

    'drivers.read',
    'drivers.manage',

    'fleets.read',
    'fleets.manage',

    'rides.create',
    'rides.read',
    'rides.update',
    'rides.cancel',

    'payments.read',
    'payments.manage',

    'profile.read',
    'profile.update'
);