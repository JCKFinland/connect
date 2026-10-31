package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/jackc/pgx/v5"
)

const (
	testOwnershipActor   = "11111111-1111-4111-8111-111111111111"
	testOwnershipDriver  = "22222222-2222-4222-8222-222222222222"
	testOwnershipCompany = "33333333-3333-4333-8333-333333333333"
	testOwnershipBinding = "44444444-4444-4444-8444-444444444444"
	testOwnershipAccount = "acct_1234567890ABCDEF"
)

type ownershipTestRow struct {
	values []string
	err    error
}

func (r ownershipTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("unexpected scan destination count")
	}
	for i, value := range r.values {
		pointer, ok := dest[i].(*string)
		if !ok {
			return errors.New("unexpected scan destination type")
		}
		*pointer = value
	}
	return nil
}

type ownershipTestDB struct {
	row   pgx.Row
	calls int
	query string
	args  []any
}

func (db *ownershipTestDB) QueryRow(
	_ context.Context,
	query string,
	args ...any,
) pgx.Row {
	db.calls++
	db.query = query
	db.args = args
	return db.row
}

func ownershipTestRequest() payouts.StripeConnectOwnershipRequest {
	return payouts.StripeConnectOwnershipRequest{
		ActorUserID:  testOwnershipActor,
		DriverUserID: testOwnershipDriver,
		CompanyID:    testOwnershipCompany,
		AccountID:    testOwnershipAccount,
	}
}

func ownershipVerifiedRow() pgx.Row {
	return ownershipTestRow{
		values: []string{
			testOwnershipBinding,
			testOwnershipActor,
			testOwnershipDriver,
			testOwnershipCompany,
			testOwnershipAccount,
		},
	}
}

func TestStripeOwnershipReaderVerified(t *testing.T) {
	db := &ownershipTestDB{row: ownershipVerifiedRow()}
	reader := NewStripeOwnershipEvidenceReader(db)
	request := ownershipTestRequest()

	evidence, err := reader.LookupVerifiedOwnership(
		context.Background(), request,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !evidence.Verified || evidence.BindingID != testOwnershipBinding {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
	if db.calls != 1 || len(db.args) != 4 {
		t.Fatalf("unexpected query invocation")
	}
	for _, condition := range []string{
		"verification_status = 'VERIFIED'",
		"revoked_at IS NULL",
		"verified_by_user_id IS NOT NULL",
		"evidence_source",
		"evidence_reference",
	} {
		if !strings.Contains(db.query, condition) {
			t.Fatalf("missing SQL safeguard: %s", condition)
		}
	}
}

func TestStripeOwnershipReaderRejectsMissingRecord(t *testing.T) {
	db := &ownershipTestDB{
		row: ownershipTestRow{err: pgx.ErrNoRows},
	}
	reader := NewStripeOwnershipEvidenceReader(db)

	evidence, err := reader.LookupVerifiedOwnership(
		context.Background(), ownershipTestRequest(),
	)
	if !errors.Is(
		err, payouts.ErrStripeConnectOwnershipNotVerified,
	) || evidence.Verified {
		t.Fatalf("expected fail-closed rejection: %v", err)
	}
}

func TestStripeOwnershipReaderRejectsMismatchedEvidence(t *testing.T) {
	row := ownershipVerifiedRow().(ownershipTestRow)
	row.values[3] = testOwnershipDriver

	db := &ownershipTestDB{row: row}
	reader := NewStripeOwnershipEvidenceReader(db)

	evidence, err := reader.LookupVerifiedOwnership(
		context.Background(), ownershipTestRequest(),
	)
	if !errors.Is(
		err, payouts.ErrStripeConnectOwnershipNotVerified,
	) || evidence.Verified {
		t.Fatalf("expected mismatched evidence rejection: %v", err)
	}
}

func TestStripeOwnershipReaderRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name   string
		change func(*payouts.StripeConnectOwnershipRequest)
	}{
		{"actor", func(r *payouts.StripeConnectOwnershipRequest) {
			r.ActorUserID = ""
		}},
		{"driver", func(r *payouts.StripeConnectOwnershipRequest) {
			r.DriverUserID = "invalid"
		}},
		{"company", func(r *payouts.StripeConnectOwnershipRequest) {
			r.CompanyID = ""
		}},
		{"account", func(r *payouts.StripeConnectOwnershipRequest) {
			r.AccountID = "invalid"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := ownershipTestRequest()
			tc.change(&request)

			db := &ownershipTestDB{row: ownershipVerifiedRow()}
			reader := NewStripeOwnershipEvidenceReader(db)

			_, err := reader.LookupVerifiedOwnership(
				context.Background(), request,
			)
			if !errors.Is(
				err, payouts.ErrStripeConnectOwnershipNotVerified,
			) || db.calls != 0 {
				t.Fatalf("expected rejection before query: %v", err)
			}
		})
	}
}

func TestStripeOwnershipReaderRejectsUnavailableDependencies(t *testing.T) {
	request := ownershipTestRequest()

	var reader *StripeOwnershipEvidenceReader
	_, err := reader.LookupVerifiedOwnership(
		context.Background(), request,
	)
	if !errors.Is(
		err, payouts.ErrStripeConnectOwnershipNotVerified,
	) {
		t.Fatalf("expected nil reader rejection: %v", err)
	}

	reader = NewStripeOwnershipEvidenceReader(nil)
	_, err = reader.LookupVerifiedOwnership(
		context.Background(), request,
	)
	if !errors.Is(
		err, payouts.ErrStripeConnectOwnershipNotVerified,
	) {
		t.Fatalf("expected nil database rejection: %v", err)
	}

	db := &ownershipTestDB{row: ownershipVerifiedRow()}
	reader = NewStripeOwnershipEvidenceReader(db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, testCtx := range []context.Context{nil, ctx} {
		_, err := reader.LookupVerifiedOwnership(testCtx, request)
		if !errors.Is(
			err, payouts.ErrStripeConnectOwnershipNotVerified,
		) {
			t.Fatalf("expected context rejection: %v", err)
		}
	}

	if db.calls != 0 {
		t.Fatalf("invalid contexts triggered database lookup")
	}
}
