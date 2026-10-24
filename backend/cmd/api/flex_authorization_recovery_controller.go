package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/JCKFinland/connect/backend/internal/config"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
)

// authorizationRecoveryLifecycle is the narrow contract required
// for supervised authorization recovery execution.
//
// Stop requests cancellation. Wait establishes actual completion.
type authorizationRecoveryLifecycle interface {
	SetOnExit(func(error)) error
	Start(
		context.Context,
		flexauthorization.AuthorizationRecoverySchedulerOptions,
	) error
	Stop()
	Wait(context.Context) error
}

// flexAuthorizationRecoveryController owns independent activation,
// supervision, and shutdown of authorization recovery.
//
// It does not construct Stripe clients or grant capture permission.
type flexAuthorizationRecoveryController struct {
	lifecycle authorizationRecoveryLifecycle
	enabled   bool

	mu       sync.Mutex
	started  bool
	stopping bool
}

func newFlexAuthorizationRecoveryController(
	cfg *config.Config,
	lifecycle authorizationRecoveryLifecycle,
) (*flexAuthorizationRecoveryController, error) {
	if cfg == nil {
		return nil, errors.New(
			"FLEX authorization recovery controller configuration is required",
		)
	}

	if err := config.ValidateFlexAuthorizationRecoveryWorkerConfiguration(
		cfg,
	); err != nil {
		return nil, fmt.Errorf(
			"validate FLEX authorization recovery activation: %w",
			err,
		)
	}

	enabled, err := cfg.FlexAuthorizationRecoveryWorker.Enabled()
	if err != nil {
		return nil, err
	}

	if enabled && lifecycle == nil {
		return nil, errors.New(
			"enabled FLEX authorization recovery requires lifecycle",
		)
	}

	return &flexAuthorizationRecoveryController{
		lifecycle: lifecycle,
		enabled:   enabled,
	}, nil
}

// Start begins recovery only when explicitly enabled.
//
// A shutdown request permanently prevents subsequent startup.
// Unexpected lifecycle exits are reported through onUnexpectedExit.
func (c *flexAuthorizationRecoveryController) Start(
	ctx context.Context,
	options flexauthorization.AuthorizationRecoverySchedulerOptions,
	onUnexpectedExit func(error),
) error {
	if c == nil {
		return errors.New(
			"FLEX authorization recovery controller is unavailable",
		)
	}

	if ctx == nil {
		return errors.New(
			"FLEX authorization recovery startup context is required",
		)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopping {
		return errors.New(
			"FLEX authorization recovery controller is stopping",
		)
	}

	if c.started {
		return errors.New(
			"FLEX authorization recovery controller already started",
		)
	}

	if !c.enabled {
		return nil
	}

	if c.lifecycle == nil {
		return errors.New(
			"FLEX authorization recovery lifecycle is unavailable",
		)
	}

	if err := c.lifecycle.SetOnExit(func(err error) {
		c.mu.Lock()
		stopping := c.stopping
		c.mu.Unlock()

		if stopping || onUnexpectedExit == nil {
			return
		}

		if err == nil {
			err = errors.New(
				"FLEX authorization recovery scheduler exited unexpectedly",
			)
		}

		onUnexpectedExit(err)
	}); err != nil {
		return fmt.Errorf(
			"configure FLEX authorization recovery supervision: %w",
			err,
		)
	}

	if err := c.lifecycle.Start(ctx, options); err != nil {
		return fmt.Errorf(
			"start FLEX authorization recovery lifecycle: %w",
			err,
		)
	}

	c.started = true
	return nil
}

// StopAndWait implements the application financial shutdown contract.
//
// Cancellation alone is not treated as completion. A context timeout
// leaves the caller responsible for waiting again before DB cleanup.
func (c *flexAuthorizationRecoveryController) StopAndWait(
	ctx context.Context,
) error {
	if c == nil {
		return errors.New(
			"FLEX authorization recovery controller is unavailable",
		)
	}

	if ctx == nil {
		return errors.New(
			"FLEX authorization recovery shutdown context is required",
		)
	}

	c.mu.Lock()
	c.stopping = true
	started := c.started
	lifecycle := c.lifecycle
	c.mu.Unlock()

	if !started || lifecycle == nil {
		return nil
	}

	lifecycle.Stop()

	if err := lifecycle.Wait(ctx); err != nil {
		return fmt.Errorf(
			"wait for FLEX authorization recovery shutdown: %w",
			err,
		)
	}

	return nil
}
