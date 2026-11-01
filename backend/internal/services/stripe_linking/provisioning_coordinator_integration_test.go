package stripelinking

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type provisioningIntegrationPermission struct {
	allowed bool
}

func (p provisioningIntegrationPermission) HasPermission(
	_ context.Context, _, permission string,
) (bool, error) {
	return p.allowed && permission == "drivers.manage", nil
}

type provisioningIntegrationMembership struct {
	allowed bool
}

func (m provisioningIntegrationMembership) Exists(
	context.Context, string, string,
) (bool, error) {
	return m.allowed, nil
}

type provisioningIntegrationDriver struct {
	driver *models.Driver
}

func (d provisioningIntegrationDriver) GetByIDForCompanyMember(
	context.Context, string, string,
) (*models.Driver, error) {
	return d.driver, nil
}

type provisioningIntegrationCreator struct {
	mu      sync.Mutex
	calls   int
	keys    []string
	account string
	err     error
}

func (p *provisioningIntegrationCreator) CreateProvisioningAccount(
	_ context.Context,
	key, actor, driver, company string,
) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.keys = append(p.keys, key)
	if key == "" || actor == "" || driver == "" || company == "" {
		return "", errors.New("missing immutable provider identity")
	}
	return p.account, p.err
}

func (p *provisioningIntegrationCreator) snapshot() (int, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls, append([]string(nil), p.keys...)
}

type provisioningIntegrationFixture struct {
	db         *pgxpool.Pool
	actor      string
	driverUser string
	company    string
}

func newProvisioningIntegrationFixture(
	t *testing.T,
) *provisioningIntegrationFixture {
	t.Helper()

	if os.Getenv("DB_NAME") != "connect_payout_destination_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "0" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("STOP: unsafe integration environment")
	}

	envOr := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
	)
	defer cancel()

	schema := "stripe_provision_test_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("DB_HOST", "localhost"),
		envOr("DB_PORT", "5432"),
		envOr("DB_USER", "postgres"),
		"connect_payout_destination_test",
	))
	if err != nil {
		t.Fatal(err)
	}

	cfg.MaxConns = 8
	cfg.AfterConnect = func(
		ctx context.Context, conn *pgx.Conn,
	) error {
		_, err := conn.Exec(
			ctx, "SET search_path TO "+schema+", public",
		)
		return err
	}

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	var safe bool
	err = db.QueryRow(ctx, `
        SELECT current_database() = 'connect_payout_destination_test'
          AND (SELECT MAX(version_id)
               FROM public.goose_db_version
               WHERE is_applied) = 75
          AND to_regclass(
              'public.stripe_connect_provisioning_intents'
          ) IS NULL
    `).Scan(&safe)
	if err != nil || !safe {
		db.Close()
		t.Fatalf("unsafe database baseline: %v %v", safe, err)
	}

	if _, err := db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		db.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		defer db.Close()
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer stop()
		if _, err := db.Exec(
			cleanupCtx, "DROP SCHEMA "+schema+" CASCADE",
		); err != nil {
			t.Errorf("isolated schema cleanup: %v", err)
		}
	})

	migration, err := os.ReadFile(
		"../../../migrations/" +
			"00079_create_stripe_connect_provisioning_intents.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	text := string(migration)
	if strings.Count(text, "-- +goose Up") != 1 ||
		strings.Count(text, "-- +goose Down") != 1 {
		t.Fatal("invalid migration sections")
	}

	up := strings.SplitN(
		strings.SplitN(text, "-- +goose Up", 2)[1],
		"-- +goose Down", 2,
	)[0]
	up = strings.ReplaceAll(up, "-- +goose StatementBegin", "")
	up = strings.ReplaceAll(up, "-- +goose StatementEnd", "")

	if _, err := db.Exec(
		ctx, up, pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("isolated migration: %v", err)
	}

	f := &provisioningIntegrationFixture{db: db}
	err = db.QueryRow(ctx, `
        SELECT
          (SELECT id::text FROM public.users
           ORDER BY id LIMIT 1),
          (SELECT id::text FROM public.users
           ORDER BY id OFFSET 1 LIMIT 1),
          (SELECT id::text FROM public.companies
           ORDER BY id LIMIT 1)
    `).Scan(&f.actor, &f.driverUser, &f.company)
	if err != nil {
		t.Fatalf("missing fixture identities: %v", err)
	}
	return f
}

func (f *provisioningIntegrationFixture) coordinator(
	creator *provisioningIntegrationCreator,
	allowed bool,
) *ProvisioningCoordinator {
	return NewProvisioningCoordinator(
		ProvisioningCoordinatorDeps{
			Permissions: provisioningIntegrationPermission{
				allowed: allowed,
			},
			Memberships: provisioningIntegrationMembership{
				allowed: true,
			},
			Drivers: provisioningIntegrationDriver{
				driver: &models.Driver{
					UserID:    f.driverUser,
					CompanyID: f.company,
					IsActive:  true,
				},
			},
			DB:       f.db,
			Accounts: creator,
		},
	)
}

func (f *provisioningIntegrationFixture) status(
	t *testing.T,
) (string, string, string) {
	t.Helper()
	var state, key string
	var account *string
	err := f.db.QueryRow(context.Background(), `
        SELECT provisioning_status,
               idempotency_key,
               stripe_account_id
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
    `, f.driverUser, f.company).Scan(
		&state, &key, &account,
	)
	if err != nil {
		t.Fatal(err)
	}
	if account == nil {
		return state, key, ""
	}
	return state, key, *account
}

func TestProvisioningCoordinatorIntegrationSuccessAndReplay(
	t *testing.T,
) {
	f := newProvisioningIntegrationFixture(t)
	creator := &provisioningIntegrationCreator{
		account: "acct_1234567890ABCDEF",
	}
	coordinator := f.coordinator(creator, true)
	profile := uuid.NewString()

	account, err := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if err != nil || account != creator.account {
		t.Fatalf("initial provisioning: %q %v", account, err)
	}

	state, key, stored := f.status(t)
	if state != "ACCOUNT_RECORDED" ||
		key == "" || stored != creator.account {
		t.Fatalf("unexpected durable state: %s %q %q",
			state, key, stored)
	}

	replay, err := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if err != nil || replay != creator.account {
		t.Fatalf("recorded replay: %q %v", replay, err)
	}

	calls, keys := creator.snapshot()
	if calls != 1 || len(keys) != 1 || keys[0] != key {
		t.Fatalf("provider called incorrectly: %d %v",
			calls, keys)
	}
}

func TestProvisioningCoordinatorIntegrationProviderUncertainty(
	t *testing.T,
) {
	f := newProvisioningIntegrationFixture(t)
	creator := &provisioningIntegrationCreator{
		err: errors.New("simulated provider timeout"),
	}
	coordinator := f.coordinator(creator, true)
	profile := uuid.NewString()

	account, err := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningReconciliationRequired) {
		t.Fatalf("provider uncertainty not fenced: %q %v",
			account, err)
	}

	state, key, stored := f.status(t)
	if state != "REQUIRES_RECONCILIATION" ||
		key == "" || stored != "" {
		t.Fatalf("unexpected uncertain state: %s %q %q",
			state, key, stored)
	}

	account, err = coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningReconciliationRequired) {
		t.Fatalf("uncertain replay not fenced: %q %v",
			account, err)
	}

	calls, keys := creator.snapshot()
	if calls != 1 || len(keys) != 1 || keys[0] != key {
		t.Fatalf("provider retried: %d %v", calls, keys)
	}
}

func TestProvisioningCoordinatorIntegrationAuthorizationDenial(
	t *testing.T,
) {
	f := newProvisioningIntegrationFixture(t)
	creator := &provisioningIntegrationCreator{
		account: "acct_1234567890ABCDEF",
	}
	coordinator := f.coordinator(creator, false)

	account, err := coordinator.ProvisionAccount(
		context.Background(), f.actor, uuid.NewString(),
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningDenied) {
		t.Fatalf("authorization bypass: %q %v", account, err)
	}

	var count int
	err = f.db.QueryRow(context.Background(), `
        SELECT COUNT(*)
        FROM stripe_connect_provisioning_intents
    `).Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("denial created ledger row: %d %v", count, err)
	}

	calls, _ := creator.snapshot()
	if calls != 0 {
		t.Fatalf("denial reached provider: %d", calls)
	}
}

// Concurrent requests must share a single durable claim and must
// never produce more than one provider account-creation attempt.
func TestProvisioningCoordinatorIntegrationConcurrentClaim(
	t *testing.T,
) {
	f := newProvisioningIntegrationFixture(t)
	creator := &provisioningIntegrationCreator{
		account: "acct_1234567890ABCDEF",
	}
	coordinator := f.coordinator(creator, true)
	profile := uuid.NewString()

	const workers = 12
	start := make(chan struct{})
	type result struct {
		account string
		err     error
	}
	results := make(chan result, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(
				context.Background(), 20*time.Second,
			)
			defer cancel()
			account, err := coordinator.ProvisionAccount(
				ctx, f.actor, profile,
			)
			results <- result{account: account, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for r := range results {
		if r.err == nil {
			if r.account != creator.account {
				t.Fatalf("unexpected account: %q", r.account)
			}
			continue
		}
		if r.account != "" ||
			!errors.Is(
				r.err, ErrStripeProvisioningReconciliationRequired,
			) {
			t.Fatalf("unexpected concurrent result: %q %v",
				r.account, r.err)
		}
	}

	state, key, stored := f.status(t)
	if state != "ACCOUNT_RECORDED" ||
		stored != creator.account || key == "" {
		t.Fatalf("invalid final ledger: %s %q %q",
			state, key, stored)
	}

	calls, keys := creator.snapshot()
	if calls != 1 || len(keys) != 1 || keys[0] != key {
		t.Fatalf("duplicate provider creation: %d %v",
			calls, keys)
	}
}

// This fake simulates cancellation during an external provider call.
// It does not contact Stripe.
type provisioningCancelCreator struct {
	mu     sync.Mutex
	calls  int
	cancel context.CancelFunc
}

func (p *provisioningCancelCreator) CreateProvisioningAccount(
	_ context.Context,
	_, _, _, _ string,
) (string, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	p.cancel()
	return "acct_1234567890ABCDEF", nil
}

func (p *provisioningCancelCreator) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestProvisioningCoordinatorIntegrationCanceledProvider(
	t *testing.T,
) {
	f := newProvisioningIntegrationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	creator := &provisioningCancelCreator{cancel: cancel}
	coordinator := NewProvisioningCoordinator(
		ProvisioningCoordinatorDeps{
			Permissions: provisioningIntegrationPermission{
				allowed: true,
			},
			Memberships: provisioningIntegrationMembership{
				allowed: true,
			},
			Drivers: provisioningIntegrationDriver{
				driver: &models.Driver{
					UserID:    f.driverUser,
					CompanyID: f.company,
					IsActive:  true,
				},
			},
			DB:       f.db,
			Accounts: creator,
		},
	)
	profile := uuid.NewString()

	account, err := coordinator.ProvisionAccount(
		ctx, f.actor, profile,
	)
	if account != "" ||
		!errors.Is(
			err, ErrStripeProvisioningReconciliationRequired,
		) {
		t.Fatalf("cancellation not fenced: %q %v",
			account, err)
	}

	state, key, stored := f.status(t)
	if state != "REQUIRES_RECONCILIATION" ||
		key == "" || stored != "" {
		t.Fatalf("invalid cancellation ledger: %s %q %q",
			state, key, stored)
	}

	replay, replayErr := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if replay != "" ||
		!errors.Is(
			replayErr, ErrStripeProvisioningReconciliationRequired,
		) {
		t.Fatalf("canceled request replay accepted: %q %v",
			replay, replayErr)
	}

	if creator.count() != 1 {
		t.Fatalf("provider invoked %d times", creator.count())
	}
}
