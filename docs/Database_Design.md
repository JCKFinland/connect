# CONNECT Database Design

## Database Engine

PostgreSQL

---

# Naming Standards

* snake_case
* Singular table names
* UUID primary keys
* Foreign key constraints
* Indexed search fields
* UTC timestamps

---

# Core Tables

## Identity

* user
* role
* permission
* user_role

---

## Driver Management

* driver
* driver_document
* driver_license
* driver_status

---

## Vehicle Management

* vehicle
* vehicle_document
* vehicle_inspection
* insurance
* taximeter

---

## Fleet Management

* fleet
* fleet_driver
* fleet_vehicle

---

## Booking

* booking
* booking_status
* booking_event

---

## Trip

* trip
* trip_location
* trip_event
* trip_route

---

## Pricing

* pricing_rule
* surcharge
* fare_estimate

---

## Payments

* payment
* refund
* receipt
* invoice

---

## Customer

* passenger
* emergency_contact
* favorite_location

---

## Ratings

* driver_rating
* passenger_rating

---

## Notifications

* notification
* notification_template

---

## Compliance

* compliance_check
* compliance_alert
* audit_log

---

## Support

* support_ticket
* support_message

---

## Reporting

* daily_summary
* monthly_summary

---

# Design Principles

* Normalize where practical.
* Enforce referential integrity.
* Optimize read-heavy workloads.
* Archive rather than delete operational records.
