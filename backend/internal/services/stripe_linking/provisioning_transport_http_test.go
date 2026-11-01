package stripelinking

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	stripe "github.com/stripe/stripe-go/v86"
)

func TestStripeAccountCreateTransportHTTPWire(t *testing.T) {
	params := validStripeTransportParams()
	expectedKey := *params.IdempotencyKey

	requests := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var failure error

			check := func(ok bool, message string) {
				if !ok && failure == nil {
					failure = fmt.Errorf("%s", message)
				}
			}

			check(r.Method == http.MethodPost, "unexpected HTTP method")
			check(r.URL.Path == "/v1/accounts", "unexpected account endpoint")
			check(
				r.Header.Get("Idempotency-Key") == expectedKey,
				"durable idempotency header mismatch",
			)
			check(
				strings.HasPrefix(
					r.Header.Get("Content-Type"),
					"application/x-www-form-urlencoded",
				),
				"unexpected content type",
			)

			if err := r.ParseForm(); err != nil {
				check(false, "unable to parse form")
			} else {
				expected := url.Values{
					"type":             {"express"},
					"country":          {"FI"},
					"default_currency": {"eur"},
				}

				for field, values := range expected {
					check(
						len(r.PostForm[field]) == 1 &&
							r.PostForm.Get(field) == values[0],
						"incorrect form field: "+field,
					)
				}

				for key, value := range params.Metadata {
					field := "metadata[" + key + "]"
					check(
						len(r.PostForm[field]) == 1 &&
							r.PostForm.Get(field) == value,
						"incorrect metadata: "+key,
					)
				}

				check(
					len(r.PostForm) == 6,
					"unexpected extra account creation fields",
				)
			}

			requests <- failure

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "acct_1234567890abcdef",
				"object":           "account",
				"type":             "express",
				"country":          "FI",
				"default_currency": "eur",
				"deleted":          false,
			})
		},
	))
	defer server.Close()

	// Reject all non-loopback destinations, even if an SDK URL
	// unexpectedly changes. No production Stripe traffic is possible.
	localClient := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(
				ctx context.Context,
				network string,
				address string,
			) (net.Conn, error) {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				ip := net.ParseIP(host)
				if ip == nil || !ip.IsLoopback() {
					return nil, fmt.Errorf(
						"STOP: non-loopback Stripe HTTP destination",
					)
				}
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, address)
			},
		},
		CheckRedirect: func(
			_ *http.Request,
			_ []*http.Request,
		) error {
			return http.ErrUseLastResponse
		},
	}

	backends := stripe.NewBackendsWithConfig(
		&stripe.BackendConfig{
			URL:               stripe.String(server.URL),
			HTTPClient:        localClient,
			MaxNetworkRetries: stripe.Int64(0),
			EnableTelemetry:   stripe.Bool(false),
		},
	)

	// Construct the real SDK client with a deliberately fake key.
	client := stripe.NewClient(
		"sk_test_connect_local_http_only",
		stripe.WithBackends(backends),
	)

	create := NewStripeAccountCreateTransport(client.V1Accounts)

	account, err := create(context.Background(), params)
	if err != nil {
		t.Fatalf("local SDK account request failed: %v", err)
	}

	if account == nil || account.ID != "acct_1234567890abcdef" {
		t.Fatalf("unexpected SDK account response: %#v", account)
	}

	select {
	case requestErr := <-requests:
		if requestErr != nil {
			t.Fatal(requestErr)
		}
	default:
		t.Fatal("local HTTP server did not receive account request")
	}
}
