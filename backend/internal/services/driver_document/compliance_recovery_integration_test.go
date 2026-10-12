package driver_document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/driver_compliance"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestDocumentVerificationRestoresEligibilityWithoutAutomaticReactivation(t *testing.T) {
	ctx := context.Background()
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(cleanupCtx,
			"DELETE FROM driver_presence WHERE driver_id = $1",
			fixture.UserID,
		); err != nil {
			t.Errorf("cleanup driver presence: %v", err)
		}

		if _, err := db.Exec(cleanupCtx,
			"DELETE FROM driver_documents WHERE driver_id = $1",
			fixture.DriverID,
		); err != nil {
			t.Errorf("cleanup driver documents: %v", err)
		}

		if err := cleanupFixture(cleanupCtx); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id, company_id, branch_id,
			vehicle_id, assignment_id,
			is_online, availability_status,
			last_heartbeat_at
		)
		VALUES ($1, $2, $3, $4, $5, FALSE, 'OFFLINE', NOW())
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
	)
	if err != nil {
		t.Fatalf("create offline presence: %v", err)
	}

	documents := postgresrepo.NewDriverDocumentRepository(db)
	drivers := postgresrepo.NewDriverRepository(db)

	documentService := NewService(Dependencies{
		DB:        db,
		Documents: documents,
	})

	compliance := driver_compliance.NewService(
		driver_compliance.Dependencies{
			Drivers:   drivers,
			Documents: documents,
		},
	)

	checkPresence := func(wantOnline bool, wantStatus string) {
		t.Helper()

		var online bool
		var status string

		err := db.QueryRow(ctx, `
			SELECT is_online, availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`, fixture.UserID).Scan(&online, &status)
		if err != nil {
			t.Fatalf("read driver presence: %v", err)
		}

		if online != wantOnline || status != wantStatus {
			t.Fatalf(
				"unexpected presence: online=%v status=%s; want online=%v status=%s",
				online, status, wantOnline, wantStatus,
			)
		}
	}

	eligible, err := compliance.IsEligible(
		ctx, fixture.DriverID, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("evaluate initial compliance: %v", err)
	}
	if eligible {
		t.Fatal("driver unexpectedly eligible without verified documents")
	}

	presenceService := presence.NewService(presence.Dependencies{
		DB:          db,
		Drivers:     drivers,
		Presence:    postgresrepo.NewDriverPresenceRepository(db),
		Assignments: postgresrepo.NewDriverAssignmentRepository(db),
		Compliance:  compliance,
	})

	err = presenceService.GoOnline(ctx, presence.GoOnlineRequest{
		UserID: fixture.UserID,
	})
	if !errors.Is(err, presence.ErrDriverComplianceRequired) {
		t.Fatalf("expected regulatory rejection before verification, got %v", err)
	}
	checkPresence(false, presence.StatusOffline)

	expiry := time.Now().UTC().AddDate(2, 0, 0)

	for _, documentType := range []string{
		models.DriverDocumentTypeDrivingLicense,
		models.DriverDocumentTypeTaxiDriverLicense,
	} {
		document := &models.DriverDocument{
			DriverID:      fixture.DriverID,
			DocumentType:  documentType,
			FileName:      "recovery-" + documentType + ".pdf",
			StorageKey:    "drivers/" + fixture.DriverID + "/recovery-" + documentType + ".pdf",
			ContentType:   "application/pdf",
			FileSizeBytes: 4096,
			ExpiresAt:     &expiry,
			Status:        models.DriverDocumentStatusPending,
		}

		if err := documents.Create(ctx, document); err != nil {
			t.Fatalf("create %s document: %v", documentType, err)
		}

		verified, err := documentService.Verify(
			ctx, document.ID, fixture.UserID,
		)
		if err != nil {
			t.Fatalf("verify %s document: %v", documentType, err)
		}
		if verified == nil ||
			verified.Status != models.DriverDocumentStatusVerified {
			t.Fatalf("document %s was not verified: %+v", documentType, verified)
		}

		// Verification must never change operational presence.
		checkPresence(false, presence.StatusOffline)
	}

	eligible, err = compliance.IsEligible(
		ctx, fixture.DriverID, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("evaluate restored compliance: %v", err)
	}
	if !eligible {
		t.Fatal("driver remains ineligible after verifying both required documents")
	}

	// Eligibility restoration alone must not reactivate the driver.
	checkPresence(false, presence.StatusOffline)

	if err := presenceService.GoOnline(
		ctx,
		presence.GoOnlineRequest{UserID: fixture.UserID},
	); err != nil {
		t.Fatalf("explicit GoOnline after regulatory recovery: %v", err)
	}

	checkPresence(true, presence.StatusAvailable)
}
