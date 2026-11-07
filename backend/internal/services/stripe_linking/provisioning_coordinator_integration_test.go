package stripelinking

import (
	"context"
	"errors"
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

	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" ||
		os.Getenv("DB_HOST") != "127.0.0.1" ||
		os.Getenv("DB_PORT") != "55432" ||
		os.Getenv("DB_USER") != "postgres" ||
		os.Getenv("DB_SSLMODE") != "disable" {
		t.Fatal("STOP: unsafe integration environment")
	}

	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		t.Fatal("STOP: isolated database password unavailable")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	defer cancel()

	schema := "stripe_provision_test_" +
		strings.ReplaceAll(uuid.NewString(), "-", "")

	cfg, err := pgxpool.ParseConfig(
		"host=127.0.0.1 port=55432 user=postgres " +
			"dbname=connect_settlement_test sslmode=disable",
	)
	if err != nil {
		t.Fatal(err)
	}

	cfg.ConnConfig.Password = password
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
        SELECT current_database() = 'connect_settlement_test'
          AND (SELECT MAX(version_id)
               FROM public.goose_db_version
               WHERE is_applied) = 79
          AND to_regclass(
              'public.stripe_connect_provisioning_intents'
          ) IS NOT NULL
    `).Scan(&safe)

	if err != nil || !safe {
		db.Close()
		t.Fatalf("unsafe database baseline: %v %v", safe, err)
	}

	if _, err := db.Exec(
		ctx, "CREATE SCHEMA "+schema,
	); err != nil {
		db.Close()
		t.Fatal(err)
	}

	f := &provisioningIntegrationFixture{db: db}

	t.Cleanup(func() {
		defer db.Close()

		var dropErr error

		for attempt := 1; attempt <= 3; attempt++ {
			cleanupCtx, stop := context.WithTimeout(
				context.Background(), 10*time.Second,
			)

			_, dropErr = db.Exec(
				cleanupCtx,
				"DROP SCHEMA "+schema+" CASCADE",
			)
			stop()

			if dropErr == nil {
				break
			}

			if attempt < 3 {
				time.Sleep(200 * time.Millisecond)
			}
		}

		if dropErr != nil {
			t.Errorf(
				"isolated schema cleanup failed: %v",
				dropErr,
			)
			return
		}

		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer stop()

		if f.company != "" {
			if _, err := db.Exec(
				cleanupCtx,
				"DELETE FROM public.companies WHERE id = $1",
				f.company,
			); err != nil {
				t.Errorf("synthetic company cleanup: %v", err)
			}
		}

		for _, id := range []string{
			f.actor,
			f.driverUser,
		} {
			if id == "" {
				continue
			}

			if _, err := db.Exec(
				cleanupCtx,
				"DELETE FROM public.users WHERE id = $1",
				id,
			); err != nil {
				t.Errorf("synthetic user cleanup: %v", err)
			}
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

	up = strings.ReplaceAll(
		up, "-- +goose StatementBegin", "",
	)
	up = strings.ReplaceAll(
		up, "-- +goose StatementEnd", "",
	)

	if _, err := db.Exec(
		ctx, up, pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("isolated migration: %v", err)
	}

	f.actor = uuid.NewString()
	f.driverUser = uuid.NewString()
	f.company = uuid.NewString()

	_, err = db.Exec(ctx, `
        INSERT INTO public.users (
            id, email, password_hash, first_name, last_name
        ) VALUES
            ($1, $2, 'synthetic-test-only', 'Test', 'Actor'),
            ($3, $4, 'synthetic-test-only', 'Test', 'Driver')
    `,
		f.actor,
		"coordinator-"+f.actor+"@example.invalid",
		f.driverUser,
		"coordinator-"+f.driverUser+"@example.invalid",
	)
	if err != nil {
		t.Fatalf("synthetic users: %v", err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO public.companies (
            id, name, legal_name, country_code, timezone
        ) VALUES (
            $1, $2, $2, 'FI', 'Europe/Helsinki'
        )
    `,
		f.company,
		"Coordinator Test "+f.company,
	)
	if err != nil {
		t.Fatalf("synthetic company: %v", err)
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
	if account != "acct_1234567890ABCDEF" || err != nil {
		t.Fatalf("cancellation not fenced: %q %v",
			account, err)
	}

	state, key, stored := f.status(t)
	if state != "ACCOUNT_RECORDED" ||
		key == "" || stored != "acct_1234567890ABCDEF" {
		t.Fatalf("invalid cancellation ledger: %s %q %q",
			state, key, stored)
	}

	replay, replayErr := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if replay != "acct_1234567890ABCDEF" ||
		replayErr != nil {
		t.Fatalf("canceled request replay accepted: %q %v",
			replay, replayErr)
	}

	if creator.count() != 1 {
		t.Fatalf("provider invoked %d times", creator.count())
	}
}

// A provider error is ambiguous even if the provider supplies an
// account ID. Never accept that ID as confirmed evidence.
func TestProvisioningCoordinatorIntegrationProviderIDWithError(
	t *testing.T,
) {
	f := newProvisioningIntegrationFixture(t)
	creator := &provisioningIntegrationCreator{
		account: "acct_1234567890ABCDEF",
		err:     errors.New("synthetic provider uncertainty"),
	}
	coordinator := f.coordinator(creator, true)
	profile := uuid.NewString()

	account, err := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningReconciliationRequired) {
		t.Fatalf("provider error accepted: %q %v", account, err)
	}

	state, key, stored := f.status(t)
	if state != "REQUIRES_RECONCILIATION" ||
		key == "" || stored != "" {
		t.Fatalf("provider error ledger: %s %q %q",
			state, key, stored)
	}

	replay, replayErr := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if replay != "" ||
		!errors.Is(replayErr, ErrStripeProvisioningReconciliationRequired) {
		t.Fatalf("provider error replay: %q %v", replay, replayErr)
	}

	calls, keys := creator.snapshot()
	if calls != 1 || len(keys) != 1 || keys[0] != key {
		t.Fatalf("duplicate provider call: %d %v", calls, keys)
	}
}

// The failure trigger lives exclusively inside the fixture's disposable
// schema. It blocks account recording, but not the reconciliation transition.
func TestProvisioningCoordinatorIntegrationRecordingFailureFence(
	t *testing.T,
) {
	f := newProvisioningIntegrationFixture(t)

	_, err := f.db.Exec(context.Background(), `
        CREATE FUNCTION reject_test_account_recording()
        RETURNS trigger
        LANGUAGE plpgsql
        AS $function$
        BEGIN
            IF NEW.provisioning_status = 'ACCOUNT_RECORDED' THEN
                RAISE EXCEPTION 'synthetic recording failure';
            END IF;
            RETURN NEW;
        END;
        $function$;
    `)
	if err != nil {
		t.Fatalf("install recording failure function: %v", err)
	}

	_, err = f.db.Exec(context.Background(), `
        CREATE TRIGGER reject_test_account_recording_trigger
        BEFORE UPDATE ON stripe_connect_provisioning_intents
        FOR EACH ROW
        EXECUTE FUNCTION reject_test_account_recording();
    `)
	if err != nil {
		t.Fatalf("install recording failure trigger: %v", err)
	}

	creator := &provisioningIntegrationCreator{
		account: "acct_1234567890ABCDEF",
	}
	coordinator := f.coordinator(creator, true)
	profile := uuid.NewString()

	account, err := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if account != "" ||
		!errors.Is(err, ErrStripeProvisioningReconciliationRequired) {
		t.Fatalf("recording failure accepted: %q %v", account, err)
	}

	state, key, stored := f.status(t)
	if state != "REQUIRES_RECONCILIATION" ||
		key == "" || stored != "" {
		t.Fatalf("recording failure ledger: %s %q %q",
			state, key, stored)
	}

	replay, replayErr := coordinator.ProvisionAccount(
		context.Background(), f.actor, profile,
	)
	if replay != "" ||
		!errors.Is(replayErr, ErrStripeProvisioningReconciliationRequired) {
		t.Fatalf("recording failure replay: %q %v", replay, replayErr)
	}

	calls, keys := creator.snapshot()
	if calls != 1 || len(keys) != 1 || keys[0] != key {
		t.Fatalf("duplicate provider call: %d %v", calls, keys)
	}
}
