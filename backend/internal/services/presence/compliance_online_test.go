package presence

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestGoOnlineRejectsNonCompliantDriverBeforePresenceMutation(
	t *testing.T,
) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	// Create an explicit OFFLINE presence row. This gives the test a
	// concrete state that must remain unchanged when compliance rejects
	// the GoOnline transition.
	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				vehicle_id,
				assignment_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				NULL,
				NULL,
				FALSE,
				'OFFLINE',
				NOW()
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_presence
				WHERE driver_id = $1
			`,
			fixture.UserID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver presence: %v",
				cleanupErr,
			)
		}
	}()

	compliance := &complianceEvaluatorStub{
		eligible: false,
	}

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance:  compliance,
		},
	)

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: fixture.UserID,
		},
	)

	if !errors.Is(
		err,
		ErrDriverComplianceRequired,
	) {
		t.Fatalf(
			"expected ErrDriverComplianceRequired, got %v",
			err,
		)
	}

	if compliance.calls != 1 {
		t.Fatalf(
			"expected compliance evaluator to be called once, got %d",
			compliance.calls,
		)
	}

	if compliance.lastDriverID != fixture.DriverID {
		t.Fatalf(
			"expected compliance evaluation for driver ID %s, got %s",
			fixture.DriverID,
			compliance.lastDriverID,
		)
	}

	var (
		isOnline           bool
		availabilityStatus string
		assignmentID       *string
		vehicleID          *string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				assignment_id,
				vehicle_id
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&assignmentID,
		&vehicleID,
	)
	if err != nil {
		t.Fatalf("load driver presence after rejection: %v", err)
	}

	if isOnline {
		t.Fatal(
			"non-compliant driver presence changed to online",
		)
	}

	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected OFFLINE availability after rejection, got %s",
			availabilityStatus,
		)
	}

	if assignmentID != nil {
		t.Fatalf(
			"expected assignment to remain detached, got %s",
			*assignmentID,
		)
	}

	if vehicleID != nil {
		t.Fatalf(
			"expected vehicle to remain detached, got %s",
			*vehicleID,
		)
	}
}

func TestGoOnlineFailsClosedWhenComplianceEvaluationFails(
	t *testing.T,
) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				FALSE,
				'OFFLINE',
				NOW()
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
	)
	if err != nil {
		t.Fatalf("create offline driver presence: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`
				DELETE FROM driver_presence
				WHERE driver_id = $1
			`,
			fixture.UserID,
		)
	}()

	compliance := failingComplianceStub()

	service := NewService(
		Dependencies{
			DB:          db,
			Config:      cfg,
			Drivers:     postgresrepo.NewDriverRepository(db),
			Presence:    postgresrepo.NewDriverPresenceRepository(db),
			Assignments: postgresrepo.NewDriverAssignmentRepository(db),
			Compliance:  compliance,
		},
	)

	err = service.GoOnline(
		ctx,
		GoOnlineRequest{
			UserID: fixture.UserID,
		},
	)

	if err == nil {
		t.Fatal(
			"expected compliance evaluation failure",
		)
	}

	if errors.Is(
		err,
		ErrDriverComplianceRequired,
	) {
		t.Fatalf(
			"expected infrastructure error, got compliance rejection: %v",
			err,
		)
	}

	var (
		isOnline           bool
		availabilityStatus string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
	)
	if err != nil {
		t.Fatalf(
			"load driver presence after compliance failure: %v",
			err,
		)
	}

	if isOnline {
		t.Fatal(
			"driver became online after compliance evaluation failure",
		)
	}

	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected OFFLINE after compliance evaluation failure, got %s",
			availabilityStatus,
		)
	}
}

func TestHeartbeatMovesNonCompliantAvailableDriverOffline(
	t *testing.T,
) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	const (
		originalLatitude  = 60.1001
		originalLongitude = 24.9001
		newLatitude       = 60.2002
		newLongitude      = 24.8002
	)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				TRUE,
				'AVAILABLE',
				$4,
				$5,
				NOW() - INTERVAL '5 minutes',
				NOW()
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		originalLatitude,
		originalLongitude,
	)
	if err != nil {
		t.Fatalf("create AVAILABLE driver presence: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`
				DELETE FROM driver_presence
				WHERE driver_id = $1
			`,
			fixture.UserID,
		)
	}()

	compliance := &complianceEvaluatorStub{
		eligible: false,
	}

	service := NewService(
		Dependencies{
			DB:         db,
			Config:     cfg,
			Drivers:    postgresrepo.NewDriverRepository(db),
			Presence:   postgresrepo.NewDriverPresenceRepository(db),
			Compliance: compliance,
		},
	)

	err = service.Heartbeat(
		ctx,
		HeartbeatRequest{
			UserID:    fixture.UserID,
			Latitude:  newLatitude,
			Longitude: newLongitude,
		},
	)

	if !errors.Is(
		err,
		ErrDriverComplianceRequired,
	) {
		t.Fatalf(
			"expected ErrDriverComplianceRequired, got %v",
			err,
		)
	}

	if compliance.calls != 1 {
		t.Fatalf(
			"expected compliance evaluator to be called once, got %d",
			compliance.calls,
		)
	}

	if compliance.lastDriverID != fixture.DriverID {
		t.Fatalf(
			"expected compliance evaluation for driver ID %s, got %s",
			fixture.DriverID,
			compliance.lastDriverID,
		)
	}

	var (
		isOnline           bool
		availabilityStatus string
		latitude           *float64
		longitude          *float64
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&latitude,
		&longitude,
	)
	if err != nil {
		t.Fatalf(
			"load driver presence after compliance rejection: %v",
			err,
		)
	}

	if isOnline {
		t.Fatal(
			"expected non-compliant AVAILABLE driver to be offline",
		)
	}

	if availabilityStatus != StatusOffline {
		t.Fatalf(
			"expected OFFLINE after compliance rejection, got %s",
			availabilityStatus,
		)
	}

	if latitude == nil || *latitude != originalLatitude {
		t.Fatalf(
			"expected rejected heartbeat to preserve latitude %v, got %v",
			originalLatitude,
			latitude,
		)
	}

	if longitude == nil || *longitude != originalLongitude {
		t.Fatalf(
			"expected rejected heartbeat to preserve longitude %v, got %v",
			originalLongitude,
			longitude,
		)
	}
}

func TestHeartbeatFailsClosedWhenComplianceEvaluationFails(
	t *testing.T,
) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	defer func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	const (
		originalLatitude  = 60.3003
		originalLongitude = 24.7003
		newLatitude       = 60.4004
		newLongitude      = 24.6004
	)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				is_online,
				availability_status,
				latitude,
				longitude,
				last_heartbeat_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				TRUE,
				'AVAILABLE',
				$4,
				$5,
				NOW() - INTERVAL '5 minutes',
				NOW()
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		originalLatitude,
		originalLongitude,
	)
	if err != nil {
		t.Fatalf("create AVAILABLE driver presence: %v", err)
	}

	defer func() {
		_, _ = db.Exec(
			context.Background(),
			`
				DELETE FROM driver_presence
				WHERE driver_id = $1
			`,
			fixture.UserID,
		)
	}()

	compliance := failingComplianceStub()

	service := NewService(
		Dependencies{
			DB:         db,
			Config:     cfg,
			Drivers:    postgresrepo.NewDriverRepository(db),
			Presence:   postgresrepo.NewDriverPresenceRepository(db),
			Compliance: compliance,
		},
	)

	err = service.Heartbeat(
		ctx,
		HeartbeatRequest{
			UserID:    fixture.UserID,
			Latitude:  newLatitude,
			Longitude: newLongitude,
		},
	)

	if err == nil {
		t.Fatal(
			"expected compliance evaluation failure",
		)
	}

	if errors.Is(
		err,
		ErrDriverComplianceRequired,
	) {
		t.Fatalf(
			"expected infrastructure error, got compliance rejection: %v",
			err,
		)
	}

	if err.Error() !=
		"evaluate driver compliance before heartbeat: compliance repository failure" {
		t.Fatalf(
			"unexpected compliance evaluation error: %v",
			err,
		)
	}

	if compliance.calls != 1 {
		t.Fatalf(
			"expected compliance evaluator to be called once, got %d",
			compliance.calls,
		)
	}

	if compliance.lastDriverID != fixture.DriverID {
		t.Fatalf(
			"expected compliance evaluation for driver ID %s, got %s",
			fixture.DriverID,
			compliance.lastDriverID,
		)
	}

	var (
		isOnline           bool
		availabilityStatus string
		latitude           *float64
		longitude          *float64
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status,
				latitude,
				longitude
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&isOnline,
		&availabilityStatus,
		&latitude,
		&longitude,
	)
	if err != nil {
		t.Fatalf(
			"load driver presence after compliance failure: %v",
			err,
		)
	}

	if !isOnline {
		t.Fatal(
			"expected AVAILABLE driver to remain online after infrastructure failure",
		)
	}

	if availabilityStatus != StatusAvailable {
		t.Fatalf(
			"expected AVAILABLE after infrastructure failure, got %s",
			availabilityStatus,
		)
	}

	if latitude == nil || *latitude != originalLatitude {
		t.Fatalf(
			"expected failed heartbeat to preserve latitude %v, got %v",
			originalLatitude,
			latitude,
		)
	}

	if longitude == nil || *longitude != originalLongitude {
		t.Fatalf(
			"expected failed heartbeat to preserve longitude %v, got %v",
			originalLongitude,
			longitude,
		)
	}
}
