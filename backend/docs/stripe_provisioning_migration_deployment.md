# CONNECT ? Stripe Provisioning Migration Deployment

## Scope

This runbook governs controlled deployment of database migrations
76 through 79.

It does not authorize Stripe provisioning activation, payout
destination verification, settlement execution, or live Stripe calls.

## Required migration sequence

1. 00076 ? Payout destination verification permission.
2. 00077 ? Stripe Connect ownership-binding ledger.
3. 00078 ? Stripe Connect account-link session ledger.
4. 00079 ? Stripe Connect provisioning intent ledger.

The expected starting migration version is 75.

The migration files must be applied in ascending order.

## Mandatory safety conditions

Before deployment:

- Confirm the intended production environment and database identity.
- Confirm the approved release commit and migration file contents.
- Confirm Stripe provisioning remains disabled.
- Confirm settlement and authorization recovery worker state
  is appropriate for the deployment window.
- Confirm an authorized deployment operator is present.
- Confirm a maintenance or change window has been approved.
- Confirm a current, restorable database backup exists.
- Confirm backup restoration has been tested in an isolated environment.
- Confirm an approved incident and recovery procedure exists.
- Confirm migration 75 is the latest applied version.
- Confirm migrations 76 through 79 are not already applied.
- Confirm the three Stripe ledger tables do not already exist.
- Confirm prerequisite users, companies, and permissions tables exist.

## Preflight

Run:

`scripts/stripe_provisioning_migration_preflight.sh`

with explicitly supplied production database connection settings,
APP_ENV=production, and
STRIPE_CONNECT_PROVISIONING_ENABLED=false.

The preflight must pass before any migration execution.

The preflight performs read-only database checks.

A passing preflight is necessary but not sufficient authorization
to modify the production database.

## Deployment authorization

Production migration execution requires a separate reviewed,
explicitly authorized deployment work order.

The deployment work order must identify:

- Exact production database target.
- Approved release commit.
- Authorized operator.
- Change window.
- Verified backup and restoration reference.
- Expected migration baseline.
- Exact migration execution mechanism.
- Post-deployment validation requirements.
- Incident response and recovery owner.

Do not infer authorization from a successful preflight.

## Migration execution policy

Apply migrations 76, 77, 78, and 79 in ascending order using
the approved Goose-compatible migration mechanism.

Do not run the application's development migration path
against production.

Do not automatically execute Goose Down.

Do not combine migration execution with Stripe provisioning
activation.

Do not automatically retry a partially failed deployment.

On failure, stop and inspect the database state before
authorizing any further action.

## Post-deployment verification

Before declaring deployment complete:

- Confirm migrations 76, 77, 78, and 79 are recorded as applied.
- Confirm no unexpected later migration was applied.
- Confirm the expected Stripe tables exist.
- Confirm their required constraints and protective triggers exist.
- Confirm the provisioning readiness schema verifier succeeds.
- Confirm Stripe provisioning remains disabled.
- Confirm no payout destination was automatically verified.
- Confirm no settlement or payout was triggered by deployment.
- Record the deployment operator, release commit, time,
  database identity, and validation results.

## Rollback and incident handling

The Down sections of migrations 77, 78, and 79 drop
Stripe ledger tables and can destroy persisted evidence.

Migration 76 Down deletes its permission record.

Never use automatic Goose Down as a routine rollback.

If deployment fails:

1. Stop further migration execution.
2. Keep Stripe provisioning disabled.
3. Preserve database and application logs.
4. Inspect the applied migration versions and schema state.
5. Escalate to the authorized incident owner.
6. Prepare a reviewed forward-repair or recovery procedure.
7. Restore from backup only under a separately approved
   incident recovery plan.

## Activation separation

Successful migration deployment does not authorize
Stripe provisioning activation.

Activation requires a separate production readiness review,
validated live-mode credentials, independent authorization,
and a distinct deployment decision.
