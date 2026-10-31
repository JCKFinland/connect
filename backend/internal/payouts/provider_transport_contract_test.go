package payouts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// transportContractLookup is a test-only implementation. It does not
// represent a production payout provider or establish real-provider safety.
func transportContractLookup(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	lookup TransferDetailsLookup,
) (VerifiedTransferDetails, error) {
	if err := ValidateTransferDetailsLookup(lookup); err != nil {
		return VerifiedTransferDetails{}, err
	}
	if ctx == nil || client == nil {
		return VerifiedTransferDetails{}, ErrInvalidRequest
	}

	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint == nil ||
		endpoint.Scheme != "http" && endpoint.Scheme != "https" ||
		endpoint.Host == "" ||
		endpoint.User != nil ||
		endpoint.RawQuery != "" ||
		endpoint.Fragment != "" {
		return VerifiedTransferDetails{}, ErrInvalidRequest
	}

	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/transfer-details"
	query := endpoint.Query()
	query.Set("idempotency_key", lookup.IdempotencyKey)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return VerifiedTransferDetails{}, err
	}

	response, err := client.Do(request)
	if err != nil {
		return VerifiedTransferDetails{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return VerifiedTransferDetails{}, fmt.Errorf(
			"provider lookup status %d: %w",
			response.StatusCode,
			ErrUnknownOutcome,
		)
	}

	var details VerifiedTransferDetails
	decoder := json.NewDecoder(
		http.MaxBytesReader(nil, response.Body, 4096),
	)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&details); err != nil {
		return VerifiedTransferDetails{}, fmt.Errorf(
			"invalid provider details: %w",
			ErrUnknownOutcome,
		)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return VerifiedTransferDetails{}, ErrUnknownOutcome
	} else if !errors.Is(err, io.EOF) {
		return VerifiedTransferDetails{}, ErrUnknownOutcome
	}

	return details, nil
}

func TestProviderTransportContractReadOnlyLookup(t *testing.T) {
	fixture := newProviderContractFixture()

	var lookupCalls int
	var submissionCalls int

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/transfer-details":
				lookupCalls++

				if r.Method != http.MethodGet {
					t.Errorf("lookup used unsafe method %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if r.ContentLength > 0 {
					t.Error("lookup unexpectedly included request body")
				}
				if r.URL.Query().Get("idempotency_key") !=
					fixture.expected.IdempotencyKey {
					t.Error("lookup key mismatch")
				}
				if len(r.URL.Query()) != 1 {
					t.Error("unexpected lookup query parameters")
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(fixture.actual)

			case "/transfers":
				submissionCalls++
				w.WriteHeader(http.StatusForbidden)

			default:
				t.Errorf("unexpected endpoint %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		},
	))
	defer server.Close()

	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(
			_ *http.Request,
			_ []*http.Request,
		) error {
			return http.ErrUseLastResponse
		},
	}

	actual, err := transportContractLookup(
		context.Background(),
		client,
		server.URL,
		TransferDetailsLookup{
			IdempotencyKey: fixture.expected.IdempotencyKey,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if actual != fixture.actual {
		t.Fatalf("unexpected provider details: %+v", actual)
	}
	if err := VerifyTransferDetails(fixture.expected, actual); err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if lookupCalls != 1 || submissionCalls != 0 {
		t.Fatalf(
			"unsafe transport calls: lookups=%d submissions=%d",
			lookupCalls,
			submissionCalls,
		)
	}
}

func TestProviderTransportContractRejectsInvalidLookup(t *testing.T) {
	client := &http.Client{Timeout: time.Second}

	_, err := transportContractLookup(
		context.Background(),
		client,
		"http://127.0.0.1:1",
		TransferDetailsLookup{IdempotencyKey: " invalid "},
	)

	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid lookup accepted: %v", err)
	}
}

func TestProviderTransportContractRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Status":`))
		},
	))
	defer server.Close()

	_, err := transportContractLookup(
		context.Background(),
		&http.Client{Timeout: time.Second},
		server.URL,
		TransferDetailsLookup{IdempotencyKey: "synthetic-key"},
	)

	if !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("malformed JSON accepted: %v", err)
	}
}

func TestProviderTransportContractRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"ProviderReference":"` +
					strings.Repeat("x", 5000) + `"}`,
			))
		},
	))
	defer server.Close()

	_, err := transportContractLookup(
		context.Background(),
		&http.Client{Timeout: time.Second},
		server.URL,
		TransferDetailsLookup{IdempotencyKey: "synthetic-key"},
	)

	if !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("oversized response accepted: %v", err)
	}
}

func TestProviderTransportContractRejectsTrailingJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{} {}`))
		},
	))
	defer server.Close()

	_, err := transportContractLookup(
		context.Background(),
		&http.Client{Timeout: time.Second},
		server.URL,
		TransferDetailsLookup{IdempotencyKey: "synthetic-key"},
	)

	if !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("trailing JSON accepted: %v", err)
	}
}

func TestProviderTransportContractRejectsRedirect(t *testing.T) {
	var redirectedCalls int

	target := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			redirectedCalls++
			w.WriteHeader(http.StatusOK)
		},
	))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(
				w, r,
				target.URL+"/transfers",
				http.StatusTemporaryRedirect,
			)
		},
	))
	defer source.Close()

	client := &http.Client{
		Timeout: time.Second,
		CheckRedirect: func(
			_ *http.Request,
			_ []*http.Request,
		) error {
			return http.ErrUseLastResponse
		},
	}

	_, err := transportContractLookup(
		context.Background(),
		client,
		source.URL,
		TransferDetailsLookup{IdempotencyKey: "synthetic-key"},
	)

	if !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("redirect accepted: %v", err)
	}

	if redirectedCalls != 0 {
		t.Fatalf("redirect unexpectedly followed: %d", redirectedCalls)
	}
}

func TestProviderTransportContractRejectsUnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"UnexpectedField":"value"}`))
		},
	))
	defer server.Close()

	_, err := transportContractLookup(
		context.Background(),
		&http.Client{Timeout: time.Second},
		server.URL,
		TransferDetailsLookup{IdempotencyKey: "synthetic-key"},
	)

	if !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("unknown JSON field accepted: %v", err)
	}
}

func TestProviderTransportContractRejectsNonSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	))
	defer server.Close()

	_, err := transportContractLookup(
		context.Background(),
		&http.Client{Timeout: time.Second},
		server.URL,
		TransferDetailsLookup{IdempotencyKey: "synthetic-key"},
	)

	if !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("non-success response accepted: %v", err)
	}
}
