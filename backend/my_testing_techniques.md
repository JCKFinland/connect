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

