# CONNECT Git Workflow

## Branches

* main
* develop
* feature/*
* release/*
* hotfix/*

---

# Feature Development

Every new feature should be developed in its own branch.

Example:

feature/authentication

feature/customer-booking

feature/driver-app

feature/admin-dashboard

---

# Commit Messages

Use clear, descriptive commit messages.

Examples:

feat: implement customer registration

fix: resolve JWT authentication issue

docs: update software requirements

refactor: simplify booking service

test: add trip service unit tests

---

# Pull Requests

Every pull request should:

* Build successfully.
* Pass tests.
* Update documentation if required.
* Be reviewed before merging.

---

# Releases

Production releases should:

* Be tagged.
* Include release notes.
* Update version numbers.
* Document breaking changes.

---

# Main Branch Policy

The main branch should always remain stable and deployable.

Unfinished work should never be merged directly into main.
