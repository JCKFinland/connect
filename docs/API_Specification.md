# CONNECT API Specification

## Version

v1

---

# API Standards

* RESTful
* JSON
* HTTPS only
* UTF-8
* Versioned endpoints

---

# Authentication

POST /api/v1/auth/register

POST /api/v1/auth/login

POST /api/v1/auth/logout

POST /api/v1/auth/refresh

GET /api/v1/auth/profile

---

# Customer

GET /api/v1/passengers

GET /api/v1/passengers/{id}

PUT /api/v1/passengers/{id}

DELETE /api/v1/passengers/{id}

---

# Driver

POST /api/v1/drivers/register

GET /api/v1/drivers/profile

PUT /api/v1/drivers/profile

POST /api/v1/drivers/status

GET /api/v1/drivers/earnings

---

# Booking

POST /api/v1/bookings

GET /api/v1/bookings

GET /api/v1/bookings/{id}

PUT /api/v1/bookings/{id}

DELETE /api/v1/bookings/{id}

---

# Trip

POST /api/v1/trips/start

POST /api/v1/trips/end

GET /api/v1/trips/history

---

# Payments

POST /api/v1/payments

GET /api/v1/payments

GET /api/v1/receipts

---

# Administration

GET /api/v1/admin/dashboard

GET /api/v1/admin/users

GET /api/v1/admin/drivers

GET /api/v1/admin/vehicles

GET /api/v1/admin/reports

---

# Response Format

{
"success": true,
"message": "",
"data": {},
"errors": []
}
