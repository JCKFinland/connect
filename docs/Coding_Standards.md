# CONNECT Coding Standards

## General Principles

* Write readable code.
* Prefer simplicity over cleverness.
* Keep functions focused on a single responsibility.
* Avoid duplicated logic.
* Write meaningful comments only where necessary.

---

# Go Standards

* Follow Effective Go conventions.
* Organize code into logical packages.
* Use dependency injection where appropriate.
* Return explicit errors.
* Never ignore errors.
* Keep handlers lightweight.
* Place business logic in service packages.

---

# React Standards

* Use functional components.
* Prefer hooks over class components.
* Keep components small and reusable.
* Separate UI from business logic.
* Use TypeScript for type safety.

---

# PostgreSQL Standards

* Use snake_case for table and column names.
* Use UUIDs for primary keys where appropriate.
* Add indexes intentionally.
* Enforce foreign keys.
* Avoid unnecessary duplication.

---

# API Standards

* Use RESTful design.
* Version APIs.
* Return consistent JSON responses.
* Use standard HTTP status codes.
* Validate every request.

---

# Logging

* Use structured logging.
* Never log passwords or secrets.
* Include request IDs when possible.
* Record significant business events.

---

# Testing

* Unit test business logic.
* Integration test APIs.
* Verify database migrations.
* Review test coverage before releases.
