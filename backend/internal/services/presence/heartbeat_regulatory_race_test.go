package presence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	drivercomplianceservice "github.com/JCKFinland/connect/backend/internal/services/driver_compliance"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestHeartbeatRejectsComplianceRevokedWhileWaitingForDriverAggregateLock(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}
	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	fixture, cleanupFixture, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupFixture(context.Background()); cleanupErr != nil {
			t.Errorf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	cleanupCompliance, err := testutil.MakeDriverRegulatorilyCompliant(ctx, db, fixture)
	if err != nil {
		t.Fatalf("make driver compliant: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupCompliance(context.Background()); cleanupErr != nil {
			t.Errorf("cleanup compliance fixture: %v", cleanupErr)
		}
	}()

	_, err = db.Exec(ctx, `
		INSERT INTO driver_presence (
			driver_id, company_id, branch_id,
			assignment_id, vehicle_id,
			is_online, availability_status,
			latitude, longitude, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7, $8, NOW())
	`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.AssignmentID,
		fixture.VehicleID,
		StatusAvailable,
		60.1001,
		24.9001,
	)
	if err != nil {
		t.Fatalf("insert available presence: %v", err)
	}
	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		)
	}()

	documentRepo := postgresrepo.NewDriverDocumentRepository(db)
	license, err := documentRepo.GetByDriverAndType(
		ctx,
		fixture.DriverID,
		models.DriverDocumentTypeDrivingLicense,
	)
	if err != nil {
		t.Fatalf("get verified driving license: %v", err)
	}

	revokeTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin regulatory transaction: %v", err)
	}
	txFinished := false
	defer func() {
		if !txFinished {
			_ = revokeTx.Rollback(context.Background())
		}
	}()

	if _, err := revokeTx.Exec(ctx, `
		SELECT id FROM drivers WHERE id = $1 FOR UPDATE
	`, fixture.DriverID); err != nil {
		t.Fatalf("lock driver aggregate: %v", err)
	}

	if _, err := revokeTx.Exec(ctx, `
		UPDATE driver_documents
		SET status = $2,
			revoked_at = NOW(),
			revoked_by_user_id = $3,
			revocation_reason = $4,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`,
		license.ID,
		models.DriverDocumentStatusRevoked,
		fixture.UserID,
		"heartbeat concurrency regression",
	); err != nil {
		t.Fatalf("stage regulatory revocation: %v", err)
	}

	service := NewService(Dependencies{
		DB:       db,
		Config:   cfg,
		Drivers:  postgresrepo.NewDriverRepository(db),
		Presence: postgresrepo.NewDriverPresenceRepository(db),
		Compliance: &complianceEvaluatorStub{
			eligible: true,
		},
		ComplianceForDB: func(dbtx postgresrepo.DBTX) ComplianceEvaluator {
			return drivercomplianceservice.NewService(
				drivercomplianceservice.Dependencies{
					Drivers:   postgresrepo.NewDriverRepositoryWithDB(dbtx),
					Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(dbtx),
				},
			)
		},
	})

	started := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		close(started)
		result <- service.Heartbeat(
			context.Background(),
			HeartbeatRequest{
				UserID:    fixture.UserID,
				Latitude:  60.2002,
				Longitude: 24.8002,
			},
		)
	}()
	<-started

	select {
	case early := <-result:
		t.Fatalf("heartbeat completed before revocation committed: %v", early)
	case <-time.After(150 * time.Millisecond):
		// Heartbeat must wait for the permanent driver aggregate lock.
	}

	if err := revokeTx.Commit(ctx); err != nil {
		t.Fatalf("commit regulatory revocation: %v", err)
	}
	txFinished = true

	select {
	case heartbeatErr := <-result:
		if !errors.Is(heartbeatErr, ErrDriverComplianceRequired) {
			t.Fatalf(
				"expected compliance rejection after revocation, got %v",
				heartbeatErr,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("heartbeat did not finish after revocation committed")
	}

	var (
		isOnline  bool
		status    string
		latitude  *float64
		longitude *float64
	)
	if err := db.QueryRow(ctx, `
		SELECT is_online, availability_status, latitude, longitude
		FROM driver_presence
		WHERE driver_id = $1
	`, fixture.UserID).Scan(
		&isOnline, &status, &latitude, &longitude,
	); err != nil {
		t.Fatalf("read presence after revocation: %v", err)
	}

	if isOnline || status != StatusOffline {
		t.Fatalf(
			"expected non-compliant driver OFFLINE, got online=%v status=%s",
			isOnline, status,
		)
	}
	if latitude == nil || *latitude != 60.1001 ||
		longitude == nil || *longitude != 24.9001 {
		t.Fatalf(
			"rejected heartbeat changed location: latitude=%v longitude=%v",
			latitude, longitude,
		)
	}
}
