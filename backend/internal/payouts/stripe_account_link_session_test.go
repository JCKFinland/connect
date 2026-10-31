package payouts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateStripeAccountLinkSession(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

	request := StripeConnectOwnershipRequest{
		ActorUserID:  uuid.NewString(),
		DriverUserID: uuid.NewString(),
		CompanyID:    uuid.NewString(),
		AccountID:    "acct_1234567890ABCDEF",
	}

	valid := StripeAccountLinkSession{
		SessionID:    uuid.NewString(),
		ActorUserID:  request.ActorUserID,
		DriverUserID: request.DriverUserID,
		CompanyID:    request.CompanyID,
		AccountID:    request.AccountID,
		CreatedAt:    now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Minute),
	}

	if err := ValidateStripeAccountLinkSession(
		context.Background(), request, valid, now,
	); err != nil {
		t.Fatalf("valid session rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*StripeAccountLinkSession)
	}{
		{"missing session ID", func(s *StripeAccountLinkSession) {
			s.SessionID = ""
		}},
		{"invalid session ID", func(s *StripeAccountLinkSession) {
			s.SessionID = "untrusted"
		}},
		{"wrong actor", func(s *StripeAccountLinkSession) {
			s.ActorUserID = uuid.NewString()
		}},
		{"wrong driver", func(s *StripeAccountLinkSession) {
			s.DriverUserID = uuid.NewString()
		}},
		{"wrong company", func(s *StripeAccountLinkSession) {
			s.CompanyID = uuid.NewString()
		}},
		{"wrong account", func(s *StripeAccountLinkSession) {
			s.AccountID = "acct_0000000000000000"
		}},
		{"missing creation", func(s *StripeAccountLinkSession) {
			s.CreatedAt = time.Time{}
		}},
		{"missing expiry", func(s *StripeAccountLinkSession) {
			s.ExpiresAt = time.Time{}
		}},
		{"invalid lifetime", func(s *StripeAccountLinkSession) {
			s.ExpiresAt = s.CreatedAt
		}},
		{"expired", func(s *StripeAccountLinkSession) {
			s.ExpiresAt = now
		}},
		{"not yet created", func(s *StripeAccountLinkSession) {
			s.CreatedAt = now.Add(time.Second)
		}},
		{"already consumed", func(s *StripeAccountLinkSession) {
			consumed := now.Add(-time.Second)
			s.ConsumedAt = &consumed
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session := valid
			tc.mutate(&session)

			err := ValidateStripeAccountLinkSession(
				context.Background(), request, session, now,
			)
			if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
				t.Fatalf("expected fail-closed rejection, got %v", err)
			}
		})
	}

	t.Run("nil context", func(t *testing.T) {
		err := ValidateStripeAccountLinkSession(
			nil, request, valid, now,
		)
		if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
			t.Fatalf("expected rejection, got %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := ValidateStripeAccountLinkSession(
			ctx, request, valid, now,
		)
		if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
			t.Fatalf("expected rejection, got %v", err)
		}
	})

	t.Run("zero current time", func(t *testing.T) {
		err := ValidateStripeAccountLinkSession(
			context.Background(), request, valid, time.Time{},
		)
		if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
			t.Fatalf("expected rejection, got %v", err)
		}
	})

	t.Run("invalid ownership request", func(t *testing.T) {
		invalid := request
		invalid.ActorUserID = ""

		err := ValidateStripeAccountLinkSession(
			context.Background(), invalid, valid, now,
		)
		if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
			t.Fatalf("expected rejection, got %v", err)
		}
	})
}
