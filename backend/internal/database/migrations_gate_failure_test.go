package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

type payoutGateTestDriver struct{}

type payoutGateTestConn struct {
	applied74     bool
	applied75     bool
	queryErr      error
	schemaMissing string
	schemaErr     error
}

type payoutGateTestRows struct {
	values []driver.Value
	read   bool
}

var payoutGateTestRegistry = struct {
	sync.Mutex
	cases map[string]payoutGateTestConn
}{
	cases: make(map[string]payoutGateTestConn),
}

func init() {
	sql.Register("connect_payout_gate_test", payoutGateTestDriver{})
}

func (payoutGateTestDriver) Open(name string) (driver.Conn, error) {
	payoutGateTestRegistry.Lock()
	defer payoutGateTestRegistry.Unlock()

	value, ok := payoutGateTestRegistry.cases[name]
	if !ok {
		return nil, errors.New("unknown payout gate test")
	}

	return &value, nil
}

func (*payoutGateTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (*payoutGateTestConn) Close() error {
	return nil
}

func (*payoutGateTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions not supported")
}

func (c *payoutGateTestConn) QueryContext(
	_ context.Context,
	query string,
	_ []driver.NamedValue,
) (driver.Rows, error) {
	if !strings.Contains(query, "goose_db_version") {
		if strings.Contains(query, "to_regclass") ||
			strings.Contains(query, "information_schema.columns") ||
			strings.Contains(query, "pg_trigger") {

			if c.schemaErr != nil {
				return nil, c.schemaErr
			}

			present := true

			if c.schemaMissing != "" &&
				strings.Contains(query, c.schemaMissing) {
				present = false
			}

			return &payoutGateTestRows{
				values: []driver.Value{present},
			}, nil
		}

		return nil, errors.New("unexpected schema query")
	}

	if !strings.Contains(query, "version_id = 74") ||
		!strings.Contains(query, "version_id = 75") {
		return nil, errors.New("unexpected migration query")
	}

	if c.queryErr != nil {
		return nil, c.queryErr
	}

	return &payoutGateTestRows{
		values: []driver.Value{
			c.applied74,
			c.applied75,
		},
	}, nil
}

func (r *payoutGateTestRows) Columns() []string {
	if len(r.values) == 1 {
		return []string{"present"}
	}

	return []string{"migration74", "migration75"}
}

func (*payoutGateTestRows) Close() error {
	return nil
}

func (r *payoutGateTestRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}

	copy(dest, r.values)
	r.read = true

	return nil
}

func TestProductionPayoutMigrationGateFailureCases(t *testing.T) {
	tests := []struct {
		name          string
		applied74     bool
		applied75     bool
		queryErr      error
		wantError     bool
		schemaMissing string
		schemaErr     error
	}{
		{
			name:      "both migrations absent",
			wantError: true,
		},
		{
			name:      "migration 74 only",
			applied74: true,
			wantError: true,
		},
		{
			name:      "migration 75 only",
			applied75: true,
			wantError: true,
		},
		{
			name:      "both migrations applied",
			applied74: true,
			applied75: true,
		},
		{
			name:      "migration history unreadable",
			queryErr:  errors.New("database unavailable"),
			wantError: true,
		},
		{
			name:          "missing payout executions table",
			applied74:     true,
			applied75:     true,
			schemaMissing: "to_regclass('public.driver_payout_executions')",
			wantError:     true,
		},
		{
			name:          "missing destination integrity trigger",
			applied74:     true,
			applied75:     true,
			schemaMissing: "trg_protect_driver_payout_destination",
			wantError:     true,
		},
		{
			name:          "missing recovery lease token",
			applied74:     true,
			applied75:     true,
			schemaMissing: "column_name = 'lease_token'",
			wantError:     true,
		},
		{
			name:      "schema verification query fails",
			applied74: true,
			applied75: true,
			schemaErr: errors.New("schema query unavailable"),
			wantError: true,
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := fmt.Sprintf("gate-case-%d", index)

			payoutGateTestRegistry.Lock()
			payoutGateTestRegistry.cases[dsn] = payoutGateTestConn{
				applied74:     tt.applied74,
				applied75:     tt.applied75,
				queryErr:      tt.queryErr,
				schemaMissing: tt.schemaMissing,
				schemaErr:     tt.schemaErr,
			}
			payoutGateTestRegistry.Unlock()

			defer func() {
				payoutGateTestRegistry.Lock()
				delete(payoutGateTestRegistry.cases, dsn)
				payoutGateTestRegistry.Unlock()
			}()

			db, err := sql.Open("connect_payout_gate_test", dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			err = verifyProductionPayoutMigrationGate(db)

			if tt.wantError && err == nil {
				t.Fatal("expected migration gate rejection")
			}

			if !tt.wantError && err != nil {
				t.Fatalf("unexpected migration gate rejection: %v", err)
			}
		})
	}
}
