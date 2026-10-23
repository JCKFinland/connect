package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFlexSettlementHealthHandlerDatabaseFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET(
		"/health",
		FlexSettlementHealthHandler(nil),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/health",
		nil,
	)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected HTTP 500, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if body.Success {
		t.Fatal("database failure must not return success")
	}

	if body.Message != "Internal server error" {
		t.Fatalf("unexpected error message: %q", body.Message)
	}

	if strings.Contains(
		strings.ToLower(recorder.Body.String()),
		"database is required",
	) {
		t.Fatal("internal database error leaked to client")
	}
}

func TestFlexSettlementHealthHandlerSnapshot(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Skip("requires isolated settlement integration database")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	dsn := "host=localhost port=5432 user=postgres dbname=connect_settlement_test sslmode=disable"

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("connect to settlement test database: %v", err)
	}

	var actualDB string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&actualDB); err != nil {
		t.Fatalf("verify database identity: %v", err)
	}

	if actualDB != "connect_settlement_test" {
		t.Fatalf("unsafe database identity: %s", actualDB)
	}

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET(
		"/health",
		FlexSettlementHealthHandler(db),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/health",
		nil,
	)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			ObservedAt               time.Time `json:"observed_at"`
			StaleAfterSeconds        int64     `json:"stale_after_seconds"`
			PendingCaptureCount      int64     `json:"pending_capture_count"`
			RecoveryCount            int64     `json:"recovery_count"`
			StalePendingCaptureCount int64     `json:"stale_pending_capture_count"`
			StaleRecoveryCount       int64     `json:"stale_recovery_count"`
		} `json:"data"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}

	if !body.Success {
		t.Fatal("snapshot response must report success")
	}

	if body.Data.ObservedAt.IsZero() {
		t.Fatal("snapshot observation timestamp is missing")
	}

	if !strings.Contains(
		recorder.Body.String(),
		`"authorization_health"`,
	) {
		t.Fatal("authorization health snapshot missing")
	}

	for _, required := range []string{
		`"processing_without_provider_count"`,
		`"processing_with_provider_count"`,
		`"stale_processing_without_provider_count"`,
		`"stale_processing_with_provider_count"`,
	} {
		if !strings.Contains(recorder.Body.String(), required) {
			t.Fatalf("missing authorization health field: %s", required)
		}
	}

	if body.Data.StaleAfterSeconds != 900 {
		t.Fatalf(
			"expected 900-second threshold, got %d",
			body.Data.StaleAfterSeconds,
		)
	}

	if body.Data.PendingCaptureCount < 0 ||
		body.Data.RecoveryCount < 0 ||
		body.Data.StalePendingCaptureCount < 0 ||
		body.Data.StaleRecoveryCount < 0 {
		t.Fatal("settlement health counts cannot be negative")
	}

	if body.Data.StalePendingCaptureCount >
		body.Data.PendingCaptureCount {
		t.Fatal("stale pending count exceeds pending count")
	}

	if body.Data.StaleRecoveryCount >
		body.Data.RecoveryCount {
		t.Fatal("stale recovery count exceeds recovery count")
	}

	for _, forbidden := range []string{
		"payment_intent",
		"provider_capture_id",
		"customer_id",
		"driver_id",
		"settlement_id",
	} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("response exposed forbidden field: %s", forbidden)
		}
	}
}
