package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

const (
	stripeProvisionTestActor   = "11111111-1111-4111-8111-111111111111"
	stripeProvisionTestDriver  = "22222222-2222-4222-8222-222222222222"
	stripeProvisionTestAccount = "acct_1234567890abcdef"
)

type stripeProvisionFake struct {
	calls   int
	actor   string
	driver  string
	account string
	err     error
}

func (f *stripeProvisionFake) ProvisionAccount(
	_ context.Context, actor, driver string,
) (string, error) {
	f.calls++
	f.actor = actor
	f.driver = driver
	return f.account, f.err
}

func TestStripeProvisioningHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name          string
		authenticated bool
		driver        string
		body          string
		unknownLength bool
		account       string
		serviceErr    error
		wantStatus    int
		wantCalls     int
	}{
		{
			name:       "missing authentication",
			driver:     stripeProvisionTestDriver,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "invalid driver",
			authenticated: true,
			driver:        "invalid",
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "client account injection rejected",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			body:          `{"stripe_account_id":"acct_injected"}`,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "client idempotency injection rejected",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			body:          `{"idempotency_key":"attacker"}`,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "unknown length body rejected",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			body:          `{"company_id":"injected"}`,
			unknownLength: true,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "malformed service account rejected",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			account:       "acct_invalid!",
			wantStatus:    http.StatusServiceUnavailable,
			wantCalls:     1,
		},
		{
			name:          "empty service account rejected",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			wantStatus:    http.StatusServiceUnavailable,
			wantCalls:     1,
		},
		{
			name:          "reconciliation fenced",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			serviceErr:    stripelinking.ErrStripeProvisioningReconciliationRequired,
			wantStatus:    http.StatusConflict,
			wantCalls:     1,
		},
		{
			name:          "authorization denied",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			serviceErr:    stripelinking.ErrStripeProvisioningDenied,
			wantStatus:    http.StatusForbidden,
			wantCalls:     1,
		},
		{
			name:          "unexpected failure unavailable",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			serviceErr:    errors.New("internal provider details"),
			wantStatus:    http.StatusServiceUnavailable,
			wantCalls:     1,
		},
		{
			name:          "validated account returned",
			authenticated: true,
			driver:        stripeProvisionTestDriver,
			account:       stripeProvisionTestAccount,
			wantStatus:    http.StatusOK,
			wantCalls:     1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &stripeProvisionFake{
				account: tc.account,
				err:     tc.serviceErr,
			}
			router := gin.New()
			router.POST(
				"/api/v1/drivers/:id/stripe-provisioning",
				func(c *gin.Context) {
					if tc.authenticated {
						user := &models.User{}
						user.ID = stripeProvisionTestActor
						middleware.SetCurrentUser(c, user)
					}
					c.Next()
				},
				NewStripeProvisioningHandler(fake).Provision,
			)

			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/drivers/"+tc.driver+"/stripe-provisioning",
				strings.NewReader(tc.body),
			)
			if tc.unknownLength {
				request.ContentLength = -1
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf(
					"status=%d want=%d body=%s",
					recorder.Code, tc.wantStatus, recorder.Body.String(),
				)
			}
			if fake.calls != tc.wantCalls {
				t.Fatalf("provider calls=%d want=%d", fake.calls, tc.wantCalls)
			}
			if fake.calls > 0 &&
				(fake.actor != stripeProvisionTestActor ||
					fake.driver != stripeProvisionTestDriver) {
				t.Fatalf("untrusted actor or driver forwarded")
			}
			if recorder.Code == http.StatusOK {
				body := recorder.Body.String()
				if !strings.Contains(body, stripeProvisionTestAccount) ||
					!strings.Contains(body, `"payouts_enabled":false`) {
					t.Fatalf("unsafe success response: %s", body)
				}
			}
			if strings.Contains(
				recorder.Body.String(), "internal provider details",
			) {
				t.Fatal("internal error details leaked")
			}
		})
	}
}
