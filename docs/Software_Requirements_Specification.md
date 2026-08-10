# Software Requirements Specification (SRS)

## Project

CONNECT – Finnish Intelligent Taxi Mobility Platform

Version: 1.0

Status: Draft

---

# 1. Purpose

This document defines the functional and non-functional requirements for the CONNECT platform.

It serves as the primary reference for system design, implementation, testing, and maintenance.

---

# 2. User Roles

The platform shall support the following roles:

* Passenger
* Driver
* Dispatcher
* Fleet Manager
* Administrator
* System Administrator
* Customer Support

---

# 3. Functional Requirements

## Passenger

The system shall allow passengers to:

* Register and authenticate.
* Manage their profile.
* Request immediate rides.
* Schedule future rides.
* Track assigned drivers.
* View fare estimates.
* Make payments.
* Download receipts.
* Rate drivers.
* Report incidents.
* View ride history.
* Save favorite locations.

---

## Driver

The system shall allow drivers to:

* Register.
* Submit verification documents.
* Manage profile.
* Manage vehicle information.
* Go online/offline.
* Accept or reject rides.
* Navigate to pickup and destination.
* Complete rides.
* View earnings.
* Access trip history.
* Report incidents.

---

## Dispatcher

The system shall allow dispatchers to:

* Monitor active drivers.
* Assign rides manually.
* Reassign rides.
* View live trips.
* Handle customer support requests.

---

## Administrator

The system shall allow administrators to:

* Manage users.
* Manage drivers.
* Manage vehicles.
* Configure pricing.
* Generate reports.
* Manage promotions.
* Review compliance alerts.
* Monitor platform health.

---

# 4. Non-Functional Requirements

The system shall provide:

* High availability.
* Secure authentication.
* Role-based authorization.
* Encrypted communication.
* Audit logging.
* Responsive user interfaces.
* Horizontal scalability.
* Comprehensive monitoring.
* Automated backups.

---

# 5. Security Requirements

The platform shall:

* Use HTTPS.
* Hash passwords securely.
* Validate all inputs.
* Enforce role-based access.
* Maintain audit logs.
* Protect sensitive data.
* Implement rate limiting.
* Support multi-factor authentication for administrators.

---

# 6. Compliance Requirements

The platform shall be designed to support:

* Applicable Finnish transport regulations.
* GDPR obligations.
* Consumer protection requirements.
* Secure payment processing.
* Operational auditability.

---

# 7. External Integrations

The platform should support integration with:

* Mapping services
* Payment providers
* SMS gateway
* Email service
* Push notification service
* Taximeter interface
* Future regulatory interfaces where available

---

# 8. Acceptance Criteria

A release is considered production-ready when:

* All critical features are implemented.
* Critical defects are resolved.
* Security testing is complete.
* Documentation is current.
* Acceptance tests pass.
* Deployment procedures are validated.
