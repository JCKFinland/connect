package driver_document

import (
	"context"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestRevokeDocumentImmediatelyOfflinesIdlePresence(t *testing.T) {
	for _, status := range []string{"AVAILABLE", "BREAK", "BUSY", "ABSENT"} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			db := openDriverDocumentTestDB(t)
			t.Cleanup(db.Close)

			fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
			if err != nil {
				t.Fatalf("create driver fixture: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx := context.Background()

				if _, err := db.Exec(
					cleanupCtx,
					"DELETE FROM driver_presence WHERE driver_id = $1",
					fixture.UserID,
				); err != nil {
					t.Errorf("cleanup driver presence: %v", err)
				}

				if _, err := db.Exec(
					cleanupCtx,
					"DELETE FROM driver_documents WHERE driver_id = $1",
					fixture.DriverID,
				); err != nil {
					t.Errorf("cleanup driver documents: %v", err)
				}

				if err := cleanupFixture(cleanupCtx); err != nil {
					t.Errorf("cleanup driver fixture: %v", err)
				}
			})

			if status != "ABSENT" {
				_, err = db.Exec(
					ctx,
					`INSERT INTO driver_presence (
                    driver_id,
                    company_id,
                    branch_id,
                    vehicle_id,
                    assignment_id,
                    is_online,
                    availability_status,
                    last_heartbeat_at
                )
                VALUES ($1, $2, $3, $4, $5, TRUE, $6, NOW())`,
					fixture.UserID,
					fixture.CompanyID,
					fixture.BranchID,
					fixture.VehicleID,
					fixture.AssignmentID,
					status,
				)
				if err != nil {
					t.Fatalf("create online presence: %v", err)
				}
			}

			documents := postgresrepo.NewDriverDocumentRepository(db)
			service := NewService(Dependencies{
				DB:        db,
				Documents: documents,
			})

			expiry := time.Now().UTC().AddDate(1, 0, 0)
			document := &models.DriverDocument{
				DriverID:      fixture.DriverID,
				DocumentType:  models.DriverDocumentTypeDrivingLicense,
				FileName:      "revocation-presence-test.pdf",
				StorageKey:    "drivers/" + fixture.DriverID + "/revocation-presence-test.pdf",
				ContentType:   "application/pdf",
				FileSizeBytes: 4096,
				ExpiresAt:     &expiry,
				Status:        models.DriverDocumentStatusPending,
			}

			if err := documents.Create(ctx, document); err != nil {
				t.Fatalf("create regulatory document: %v", err)
			}

			if _, err := service.Verify(
				ctx,
				document.ID,
				fixture.UserID,
			); err != nil {
				t.Fatalf("verify regulatory document: %v", err)
			}

			revoked, err := service.Revoke(
				ctx,
				document.ID,
				fixture.UserID,
				"regulatory credential withdrawn",
			)
			if err != nil {
				t.Fatalf("revoke regulatory document: %v", err)
			}

			if revoked == nil ||
				revoked.Status != models.DriverDocumentStatusRevoked {
				t.Fatalf("expected revoked document, got %+v", revoked)
			}

			if status == "ABSENT" {
				var count int
				err := db.QueryRow(
					ctx,
					"SELECT COUNT(*) FROM driver_presence WHERE driver_id = $1",
					fixture.UserID,
				).Scan(&count)
				if err != nil {
					t.Fatalf("check absent presence: %v", err)
				}
				if count != 0 {
					t.Fatalf("expected no presence row, found %d", count)
				}
				return
			}

			var isOnline bool
			var availabilityStatus string

			err = db.QueryRow(
				ctx,
				`SELECT is_online, availability_status
                 FROM driver_presence
                 WHERE driver_id = $1`,
				fixture.UserID,
			).Scan(&isOnline, &availabilityStatus)
			if err != nil {
				t.Fatalf("read presence after revocation: %v", err)
			}

			expectedOnline := status == "BUSY"
			expectedStatus := "OFFLINE"
			if status == "BUSY" {
				expectedStatus = "BUSY"
			}

			if isOnline != expectedOnline ||
				availabilityStatus != expectedStatus {
				t.Fatalf(
					"unexpected presence after revocation: initial=%s online=%v status=%s; want online=%v status=%s",
					status,
					isOnline,
					availabilityStatus,
					expectedOnline,
					expectedStatus,
				)
			}
		})
	}
}

func TestRevokeDocumentRollsBackWhenPresenceUpdateFails(t *testing.T) {
	ctx := context.Background()
	db := openDriverDocumentTestDB(t)
	defer db.Close()

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(
			cleanupCtx,
			"DELETE FROM driver_presence WHERE driver_id = $1",
			fixture.UserID,
		); err != nil {
			t.Errorf("cleanup presence: %v", err)
		}

		if _, err := db.Exec(
			cleanupCtx,
			"DELETE FROM driver_documents WHERE driver_id = $1",
			fixture.DriverID,
		); err != nil {
			t.Errorf("cleanup documents: %v", err)
		}

		if err := cleanupFixture(cleanupCtx); err != nil {
			t.Errorf("cleanup fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`INSERT INTO driver_presence (
            driver_id, company_id, branch_id, vehicle_id,
            assignment_id, is_online, availability_status,
            last_heartbeat_at
        )
        VALUES ($1, $2, $3, $4, $5, TRUE, 'AVAILABLE', NOW())`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("insert presence: %v", err)
	}

	documents := postgresrepo.NewDriverDocumentRepository(db)
	service := NewService(Dependencies{
		DB:        db,
		Documents: documents,
	})

	expiry := time.Now().UTC().AddDate(1, 0, 0)
	document := &models.DriverDocument{
		DriverID:      fixture.DriverID,
		DocumentType:  models.DriverDocumentTypeDrivingLicense,
		FileName:      "revocation-rollback-test.pdf",
		StorageKey:    "drivers/" + fixture.DriverID + "/revocation-rollback-test.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiry,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := documents.Create(ctx, document); err != nil {
		t.Fatalf("create document: %v", err)
	}

	if _, err := service.Verify(
		ctx, document.ID, fixture.UserID,
	); err != nil {
		t.Fatalf("verify document: %v", err)
	}

	// A CHECK constraint on a temporary shadow table would not affect
	// production queries. Instead, install a narrowly scoped trigger on
	// the real presence table and remove it before cleanup.
	_, err = db.Exec(
		ctx,
		`CREATE OR REPLACE FUNCTION connect_test_revoke_presence_failure()
         RETURNS trigger AS $$
         BEGIN
             IF NEW.driver_id = '`+fixture.UserID+`'::uuid
                AND NEW.availability_status = 'OFFLINE'
                AND OLD.availability_status = 'AVAILABLE' THEN
                 RAISE EXCEPTION 'injected presence reconciliation failure';
             END IF;
             RETURN NEW;
         END;
         $$ LANGUAGE plpgsql`,
	)
	if err != nil {
		t.Fatalf("create failure function: %v", err)
	}

	defer func() {
		if _, err := db.Exec(
			context.Background(),
			`DROP FUNCTION IF EXISTS connect_test_revoke_presence_failure() CASCADE`,
		); err != nil {
			t.Errorf("remove failure function: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`CREATE TRIGGER connect_test_revoke_presence_failure_trigger
         BEFORE UPDATE ON driver_presence
         FOR EACH ROW
         EXECUTE FUNCTION connect_test_revoke_presence_failure()`,
	)
	if err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	revoked, err := service.Revoke(
		ctx,
		document.ID,
		fixture.UserID,
		"rollback test",
	)
	if err == nil {
		t.Fatalf("expected injected presence update failure, got document %+v", revoked)
	}

	var documentStatus string
	err = db.QueryRow(
		ctx,
		"SELECT status FROM driver_documents WHERE id = $1",
		document.ID,
	).Scan(&documentStatus)
	if err != nil {
		t.Fatalf("read document after rollback: %v", err)
	}

	if documentStatus != string(models.DriverDocumentStatusVerified) {
		t.Fatalf("revocation was not rolled back: status=%s", documentStatus)
	}

	var isOnline bool
	var presenceStatus string
	err = db.QueryRow(
		ctx,
		`SELECT is_online, availability_status
         FROM driver_presence WHERE driver_id = $1`,
		fixture.UserID,
	).Scan(&isOnline, &presenceStatus)
	if err != nil {
		t.Fatalf("read presence after rollback: %v", err)
	}

	if !isOnline || presenceStatus != "AVAILABLE" {
		t.Fatalf(
			"presence changed despite rollback: online=%v status=%s",
			isOnline, presenceStatus,
		)
	}
}
