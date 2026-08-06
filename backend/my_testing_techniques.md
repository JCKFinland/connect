# CONNECT Backend API Test Guide (Milestone 4)
Base URL: http://localhost:8000/api/v1


# Test 1 — Health Check
GET
/api/v1/health

Headers: None
Body: None

Expected Response
Status: 200 OK

Example:
{
  "success": true,
  "message": "API is healthy"
}


# Test 2 — Register User
Method: POST
URL: /api/v1/auth/register

Body:

{
  "email": "john@example.com",
  "password": "Password123!",
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+358401234567"
}

Expected Response:
Status: 201 Created
Example:
{
  "success": true,
  "message": "User registered successfully"
}


# Test 3 — Login
Request
Method: POST
URL:/api/v1/auth/login

Headers:
Content-Type  application/json

Body:
{
  "email": "john@example.com",
  "password": "Password123!"
}

Expected Response:

{
  "success": true,
  "message": "Login successful",
  "data": {
    "access_token": "...",
    "refresh_token": "...",
    "token_type": "Bearer",
    "expires_in": 900
  }
}

Save These Tokens:
Access Token:
Bearer eyJhbGciOi...
Refresh Token:
abc123xyz...

You'll use them throughout the remaining tests.

# Test 4 — Current User
Request
Method: GET
URL: /api/v1/users/me

Headers:

Authorization          Bearer YOUR_ACCESS_TOKEN

Example: Authorization: Bearer eyJhbGc...

Body: None

Expected Response:

{
  "success": true,
  "message": "User profile",
  "data": {
    "id": "...",
    "email": "john@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "phone": "+358401234567",
    "is_active": true,
    "is_verified": false
  }
}

# est 5 — Refresh Token

Request
Method:POST
URL: /api/v1/auth/refresh

Headers:

Content-Type     application/json

Body:

{
  "refresh_token": "YOUR_REFRESH_TOKEN"
}


Expected Response:

{
  "success": true,
  "message": "Token refreshed successfully",
  "data": {
    "access_token": "...",
    "refresh_token": "...",
    "token_type": "Bearer",
    "expires_in": 900
  }
}

# IMPORTANT

The refresh token has now rotated.

Discard the old refresh token.

Use only the new one returned above.

# Test 6 — Verify New Access Token
NOTE: Use the new access token.

Method: GET

URL: /api/v1/users/me
Headers: Authorization: Bearer NEW_ACCESS_TOKEN

Expected: 200 OK

# Test 7 — Logout

Method:POST

URL: /api/v1/auth/logout

Headers:

Content-Type      application/json

BODY:

{
  "refresh_token": "NEW_REFRESH_TOKEN"
}

Expected:

{
  "success": true,
  "message": "Logout successful"
}


# Test 8 — Verify Logout
Immediately call Refresh again

Method:POST

URL: /api/v1/auth/refresh

Headers:

Content-Type application/json

BODY:

 {
  "refresh_token": "THE_SAME_REFRESH_TOKEN_USED_FOR_LOGOUT"
}

Expected:

{
  "success": false,
  "message": "invalid or expired refresh token"
}

This confirms Logout revoked the refresh token correctly.

# Test 9 — Unauthorized Request

Method:GET

URL: /api/v1/users/me

Headers: None

Expected: 401 Unauthorized


# Test 10 — Invalid JWT

Method:GET

URL: /api/v1/users/me

Headers:

Authorization: Bearer abc123

Expected: 401 Unauthorized


# Test 11 — RBAC Endpoint

First, log in again to obtain a fresh access token.

Method: GET

URL: /api/v1/admin/ping

Headers: Authorization: Bearer YOUR_ACCESS_TOKEN

Body: None

Expected if the user has permission:

{
  "success": true,
  "message": "RBAC working"
}


Expected if permission is missing:

{
  "success": false,
  "message": "Insufficient permissions"
}


# SQL Verification Queries

Check User:

SELECT
    id,
    email,
    is_active,
    is_verified
FROM users;

Check Roles:

SELECT
    id,
    name
FROM roles;

Check User Roles:

SELECT
    u.email,
    r.name
FROM users u
JOIN user_roles ur
    ON ur.user_id = u.id
JOIN roles r
    ON r.id = ur.role_id;


Check Permissions
 SELECT
    r.name,
    p.name
FROM roles r
JOIN role_permissions rp
    ON rp.role_id = r.id
JOIN permissions p
    ON p.id = rp.permission_id
ORDER BY r.name, p.name;


Check Refresh Tokens

SELECT
    user_id,
    expires_at,
    revoked_at,
    created_at
FROM refresh_tokens;


Delete All Refresh Tokens (Development Only)

DELETE FROM refresh_tokens;


# Create Branch

POST /api/v1/branches

BODY
{
  "company_id": "YOUR_COMPANY_UUID",
  "code": "HQ",
  "name": "Helsinki Head Office",
  "email": "helsinki@connect.fi",
  "phone": "+358401234567",
  "address_line1": "Keilaranta 1",
  "address_line2": "",
  "city": "Espoo",
  "state": "Uusimaa",
  "postal_code": "02150",
  "latitude": 60.1719,
  "longitude": 24.941,
  "is_active": true
}


# List Branches
GET /api/v1/branches

Expected:

HTTP 200
Array containing the branch


# Get Branch
GET /api/v1/branches/{id}

Expected:

HTTP 200
Correct branch object

# Update Branch
PUT /api/v1/branches/{id}

Example:
{
  "code": "HQ",
  "name": "CONNECT Headquarters",
  "email": "hq@connect.fi",
  "phone": "+358409999999",
  "address_line1": "Keilaranta 1",
  "address_line2": "Building B",
  "city": "Espoo",
  "state": "Uusimaa",
  "postal_code": "02150",
  "latitude": 60.1719,
  "longitude": 24.941,
  "is_active": true
}

# Delete Branch
DELETE /api/v1/branches/{id}

Then verify the soft delete:

SELECT
    id,
    name,
    deleted_at
FROM branches;

You should see a non-NULL deleted_at timestamp.

# How to find company's ID
SELECT
    id,
    name,
    legal_name
FROM companies
WHERE deleted_at IS NULL;

                  id                  |   name   | legal_name
--------------------------------------+----------+------------
c4cf9c29-f6fb-46bd-9ad9-65c576f9b990  | CONNECT  | CONNECT

NOTE: The value under id is your company UUID.


# Include Soft-Deleted Companies
# To see every company:

SELECT
    id,
    name,
    deleted_at
FROM companies;

Example:
                  id                  |  name   | deleted_at
--------------------------------------+---------+-------------------------------
c4cf9c29-f6fb-46bd-9ad9-65c576f9b990  | CONNECT | 2026-08-02 14:40:30.475622+03

NOTE: If deleted_at has a timestamp, that company has been soft-deleted.


# How to see every company Via Your API
GET /api/v1/companies

The response includes:

{
  "data": [
    {
      "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
      "name": "CONNECT"
    }
  ]
}

# Important: For testing Branch creation, the company must exist and not be deleted.

You can create company via API
POST /api/v1/companies

Then copy the new UUID from the response:

{
    "id": "NEW-COMPANY-UUID"
}

Use that UUID in your Branch creation request:

{
  "company_id": "NEW-COMPANY-UUID",
  ...
}

# NOTE: This ensures the branch references an active company, which is the correct setup for subsequent modules like Fleet and Vehicle.




# Functional Testing

1. Create Driver
POST /api/v1/drivers

Headers:
Authorization: Bearer <JWT_TOKEN>
Content-Type: application/json

Body:
{
  "user_id": "YOUR_USER_ID",
  "company_id": "YOUR_COMPANY_ID",
  "branch_id": "YOUR_BRANCH_ID",
  "driver_number": "DRV-0001",
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+358401234567",
  "email": "john.doe@example.com",
  "taxi_driver_license_number": "TDL-2026-001",
  "driving_license_number": "B12345678",
  "status": "ACTIVE",
  "is_verified": true,
  "is_active": true
}

Expected: 201 Created


2. List Drivers

GET /api/v1/drivers

Expected: 200 OK


3. Get Driver

GET /api/v1/drivers/{driver_id}

Expected: 200 OK


4. Update Driver

PUT /api/v1/drivers/{driver_id}

Example:
{
  "phone": "+358401112222",
  "email": "driver.updated@example.com",
  "status": "ON_DUTY",
  "is_verified": true,
  "is_active": true
}

Expected: 200 OK

5. Delete Driver

DELETE /api/v1/drivers/{driver_id}

Expected: 200 OK

6. Verify Soft Delete

GET /api/v1/drivers

The deleted driver should no longer appear.

# Database Verification
Run:
SELECT
    id,
    driver_number,
    first_name,
    last_name,
    status,
    deleted_at
FROM drivers;

You should see the deleted_at timestamp populated for the deleted driver.


# HOW TO GET IDS

1. Get user_id

GET /api/v1/users

OR

SELECT
    id,
    email
FROM users;


2. Get company_id

GET /api/v1/companies

OR

SELECT
    id,
    name
FROM companies
WHERE deleted_at IS NULL;


3. Get branch_id

GET /api/v1/branches

OR

SELECT
    id,
    company_id,
    name
FROM branches
WHERE deleted_at IS NULL;



4. Get fleet_id

GET /api/v1/fleets

OR

SELECT
    id,
    name
FROM fleets
WHERE deleted_at IS NULL;


5. Get vehicle_id

GET /api/v1/vehicles

OR

SELECT
    id,
    registration_number
FROM vehicles
WHERE deleted_at IS NULL;

6. Get driver_id
After you create a driver:

GET /api/v1/drivers

OR

SELECT
    id,
    driver_number,
    first_name,
    last_name
FROM drivers
WHERE deleted_at IS NULL;


# Quick PostgreSQL Queries

These are the ones you'll use most often during development:

-- Users
SELECT id, email FROM users;

-- Companies
SELECT id, name FROM companies WHERE deleted_at IS NULL;

-- Branches
SELECT id, name FROM branches WHERE deleted_at IS NULL;

-- Fleets
SELECT id, name FROM fleets WHERE deleted_at IS NULL;

-- Vehicles
SELECT id, registration_number FROM vehicles WHERE deleted_at IS NULL;

-- Drivers
SELECT id, driver_number, first_name, last_name
FROM drivers
WHERE deleted_at IS NULL;



# Functional Testing

1. Create Driver-Vehicle Assignment.

POST /api/v1/driver-vehicle-assignments

Headers:
Authorization: Bearer <JWT>

Content-Type: application/json

Body:

{
  "company_id": "<company_id>",
  "branch_id": "<branch_id>",
  "fleet_id": "<fleet_id>",
  "driver_id": "<driver_id>",
  "vehicle_id": "<vehicle_id>",
  "assigned_by": "<user_id>",
  "notes": "Morning shift assignment"
}

Expected:
201 Created

# 2. List Assignments

GET /api/v1/driver-vehicle-assignments

Expected:
200 OK

# 3. Get Assignment

GET /api/v1/driver-vehicle-assignments/{assignment_id}

Expected:
200 OK

# 4. Release Assignment

PATCH /api/v1/driver-vehicle-assignments/{assignment_id}/release

Expected:
200 OK

Then verify:

status = RELEASED
is_active = false
released_at != null

# 5. Delete Assignment

DELETE /api/v1/driver-vehicle-assignments/{assignment_id}

Expected:
200 OK

# Database Verification

After testing:

SELECT
    id,
    driver_id,
    vehicle_id,
    status,
    is_active,
    assigned_at,
    released_at
FROM driver_vehicle_assignments;

You should observe:

Active assignments have:
status = ACTIVE
is_active = true
released_at = NULL
Released assignments have:
status = RELEASED
is_active = false
released_at populated




