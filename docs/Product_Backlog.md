# CONNECT Product Backlog

**Project:** CONNECT – Finnish Intelligent Taxi Mobility Platform

**Version:** 1.0

**Status:** Active

**Owner:** Product Team

---

# Purpose

The CONNECT Product Backlog is the master list of all work required to design, build, test, deploy, and maintain the CONNECT platform.

Every feature, enhancement, bug fix, technical improvement, security requirement, and compliance requirement shall be tracked within this backlog.

The backlog is a living document and will evolve throughout the lifecycle of the project.

---

# Priority Levels

| Priority | Description                          |
| -------- | ------------------------------------ |
| Critical | Required before production release   |
| High     | Required for MVP                     |
| Medium   | Important but may be scheduled later |
| Low      | Optional improvement                 |
| Future   | Planned for future releases          |

---

# Status

| Status      | Description                 |
| ----------- | --------------------------- |
| Not Started | Work has not begun          |
| In Progress | Currently being implemented |
| Review      | Awaiting review             |
| Testing     | Under testing               |
| Completed   | Finished                    |
| Blocked     | Waiting for dependency      |

---

# Epic 1 – Project Foundation

| ID       | Feature                      | Priority | Status      |
| -------- | ---------------------------- | -------- | ----------- |
| PROJ-001 | Create repository            | Critical | Completed   |
| PROJ-002 | Define folder structure      | Critical | Completed   |
| PROJ-003 | Create project documentation | Critical | Completed   |
| PROJ-004 | Define coding standards      | Critical | Completed   |
| PROJ-005 | Define Git workflow          | Critical | Completed   |
| PROJ-006 | Define architecture          | Critical | Completed   |
| PROJ-007 | Create SRS                   | Critical | Completed   |
| PROJ-008 | Create roadmap               | High     | Completed   |
| PROJ-009 | Create ADR framework         | High     | Completed   |
| PROJ-010 | Create product backlog       | Critical | In Progress |

---

# Epic 2 – Backend Foundation

| ID       | Feature                          | Priority | Status      |
| -------- | -------------------------------- | -------- | ----------- |
| CORE-001 | Initialize Go module             | Critical | Not Started |
| CORE-002 | Configure Gin                    | Critical | Not Started |
| CORE-003 | Create application configuration | Critical | Not Started |
| CORE-004 | Environment variable support     | Critical | Not Started |
| CORE-005 | Structured logging               | High     | Not Started |
| CORE-006 | PostgreSQL connection            | Critical | Not Started |
| CORE-007 | Database migration framework     | Critical | Not Started |
| CORE-008 | Health check endpoint            | Critical | Not Started |
| CORE-009 | Graceful shutdown                | High     | Not Started |
| CORE-010 | Global error handling            | High     | Not Started |

---

# Epic 3 – Authentication & Authorization

| ID       | Feature                             | Priority | Status      |
| -------- | ----------------------------------- | -------- | ----------- |
| AUTH-001 | User registration                   | Critical | Not Started |
| AUTH-002 | User login                          | Critical | Not Started |
| AUTH-003 | JWT authentication                  | Critical | Not Started |
| AUTH-004 | Refresh tokens                      | High     | Not Started |
| AUTH-005 | Password reset                      | Medium   | Not Started |
| AUTH-006 | Email verification                  | Medium   | Not Started |
| AUTH-007 | Multi-factor authentication (Admin) | High     | Not Started |
| AUTH-008 | Role-based access control (RBAC)    | Critical | Not Started |

---

# Epic 4 – Customer Platform

| ID       | Feature               | Priority | Status      |
| -------- | --------------------- | -------- | ----------- |
| CUST-001 | Customer registration | Critical | Not Started |
| CUST-002 | Customer profile      | High     | Not Started |
| CUST-003 | Book immediate ride   | Critical | Not Started |
| CUST-004 | Schedule ride         | High     | Not Started |
| CUST-005 | Ride history          | Medium   | Not Started |
| CUST-006 | Favourite locations   | Medium   | Not Started |
| CUST-007 | Fare estimate         | Critical | Not Started |
| CUST-008 | Driver tracking       | Critical | Not Started |
| CUST-009 | Digital receipts      | High     | Not Started |
| CUST-010 | Emergency SOS         | High     | Not Started |

---

# Epic 5 – Driver Platform

| ID       | Feature                 | Priority | Status      |
| -------- | ----------------------- | -------- | ----------- |
| DRVR-001 | Driver registration     | Critical | Not Started |
| DRVR-002 | Driver verification     | Critical | Not Started |
| DRVR-003 | Vehicle registration    | Critical | Not Started |
| DRVR-004 | Online / Offline status | Critical | Not Started |
| DRVR-005 | Accept ride             | Critical | Not Started |
| DRVR-006 | Reject ride             | High     | Not Started |
| DRVR-007 | Navigation              | High     | Not Started |
| DRVR-008 | Earnings dashboard      | High     | Not Started |
| DRVR-009 | Trip history            | Medium   | Not Started |
| DRVR-010 | Document management     | High     | Not Started |

---

# Epic 6 – Dispatch Engine

| ID       | Feature            | Priority | Status      |
| -------- | ------------------ | -------- | ----------- |
| DISP-001 | Automatic dispatch | Critical | Not Started |
| DISP-002 | Manual dispatch    | Critical | Not Started |
| DISP-003 | Driver assignment  | Critical | Not Started |
| DISP-004 | Scheduled dispatch | High     | Not Started |
| DISP-005 | Queue management   | High     | Not Started |
| DISP-006 | Fleet assignment   | Medium   | Not Started |

---

# Epic 7 – Admin Portal

| ID        | Feature             | Priority | Status      |
| --------- | ------------------- | -------- | ----------- |
| ADMIN-001 | Dashboard           | Critical | Not Started |
| ADMIN-002 | Live monitoring     | Critical | Not Started |
| ADMIN-003 | Driver management   | Critical | Not Started |
| ADMIN-004 | Vehicle management  | High     | Not Started |
| ADMIN-005 | Customer management | High     | Not Started |
| ADMIN-006 | Pricing management  | High     | Not Started |
| ADMIN-007 | Reporting           | High     | Not Started |
| ADMIN-008 | Audit logs          | High     | Not Started |

---

# Epic 8 – Payments

| ID      | Feature            | Priority | Status      |
| ------- | ------------------ | -------- | ----------- |
| PAY-001 | Payment processing | Critical | Not Started |
| PAY-002 | Payment history    | High     | Not Started |
| PAY-003 | Digital receipts   | Critical | Not Started |
| PAY-004 | Refund processing  | Medium   | Not Started |

---

# Epic 9 – Notifications

| ID        | Feature              | Priority | Status      |
| --------- | -------------------- | -------- | ----------- |
| NOTIF-001 | Push notifications   | High     | Not Started |
| NOTIF-002 | SMS notifications    | Medium   | Not Started |
| NOTIF-003 | Email notifications  | Medium   | Not Started |
| NOTIF-004 | In-app notifications | High     | Not Started |

---

# Epic 10 – Reporting & Analytics

| ID      | Feature            | Priority | Status      |
| ------- | ------------------ | -------- | ----------- |
| RPT-001 | Daily reports      | High     | Not Started |
| RPT-002 | Revenue reports    | High     | Not Started |
| RPT-003 | Driver analytics   | High     | Not Started |
| RPT-004 | Fleet analytics    | Medium   | Not Started |
| RPT-005 | Compliance reports | High     | Not Started |

---

# Epic 11 – Compliance

| ID       | Feature                     | Priority | Status      |
| -------- | --------------------------- | -------- | ----------- |
| COMP-001 | Driver licence verification | Critical | Not Started |
| COMP-002 | Vehicle verification        | Critical | Not Started |
| COMP-003 | Operator verification       | High     | Not Started |
| COMP-004 | Compliance dashboard        | High     | Not Started |
| COMP-005 | Audit trail                 | Critical | Not Started |
| COMP-006 | GDPR requests               | High     | Not Started |

---

# Epic 12 – MiTax-400 Integration

| ID        | Feature                         | Priority | Status      |
| --------- | ------------------------------- | -------- | ----------- |
| MITAX-001 | Research communication protocol | High     | Not Started |
| MITAX-002 | Design abstraction layer        | High     | Not Started |
| MITAX-003 | Device connection               | High     | Not Started |
| MITAX-004 | Trip synchronization            | High     | Not Started |
| MITAX-005 | Fare synchronization            | High     | Not Started |
| MITAX-006 | Error recovery                  | Medium   | Not Started |

---

# Epic 13 – Testing & Quality Assurance

| ID       | Feature                 | Priority | Status      |
| -------- | ----------------------- | -------- | ----------- |
| TEST-001 | Unit testing            | Critical | Not Started |
| TEST-002 | Integration testing     | Critical | Not Started |
| TEST-003 | API testing             | High     | Not Started |
| TEST-004 | Performance testing     | High     | Not Started |
| TEST-005 | Security testing        | Critical | Not Started |
| TEST-006 | User acceptance testing | High     | Not Started |

---

# Epic 14 – Production Deployment

| ID       | Feature               | Priority | Status      |
| -------- | --------------------- | -------- | ----------- |
| PROD-001 | Docker deployment     | High     | Not Started |
| PROD-002 | CI/CD pipeline        | High     | Not Started |
| PROD-003 | Production monitoring | High     | Not Started |
| PROD-004 | Automated backups     | High     | Not Started |
| PROD-005 | Disaster recovery     | Medium   | Not Started |
| PROD-006 | Production release    | Critical | Not Started |

---

# Notes

* Every backlog item shall be traceable to the Software Requirements Specification (SRS).
* Every completed feature shall have associated documentation and tests.
* Regulatory and compliance-related work shall be prioritized alongside functional development.
* The backlog shall be reviewed and updated at the beginning and end of every development milestone.
