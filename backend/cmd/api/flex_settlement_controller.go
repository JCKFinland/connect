package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
)

// flexSettlementLifecycle is the narrow interface needed
// for startup, supervision, and graceful shutdown.
type flexSettlementLifecycle interface {
	Start(context.Context, flexsettlement.DiscoveryWorkerOptions) error
	SetOnExit(func(error)) error
	Stop()
	Wait(context.Context) error
}

// flexSettlementController owns one worker lifecycle.
// Construction alone never starts financial processing.
type flexSettlementController struct {
	lifecycle flexSettlementLifecycle
	enabled   bool

	mu       sync.Mutex
	started  bool
	stopping bool
}

// newFlexSettlementController validates the activation policy.
// A disabled controller requires no worker dependencies.
func newFlexSettlementController(
	cfg *config.Config,
	lifecycle flexSettlementLifecycle,
) (*flexSettlementController, error) {
	if err := config.ValidateFlexSettlementWorkerConfiguration(
		cfg,
	); err != nil {
		return nil, err
	}

	enabled, err := cfg.FlexSettlementWorker.Enabled()
	if err != nil {
		return nil, err
	}

	if enabled && lifecycle == nil {
		return nil, errors.New(
			"enabled FLEX settlement worker requires lifecycle",
		)
	}

	return &flexSettlementController{
		lifecycle: lifecycle,
		enabled:   enabled,
	}, nil
}

// Start performs no work when financial discovery is disabled.
func (c *flexSettlementController) Start(
	ctx context.Context,
	options flexsettlement.DiscoveryWorkerOptions,
	onUnexpectedExit func(error),
) error {
	if c == nil {
		return errors.New("FLEX settlement controller is nil")
	}

	if !c.enabled {
		return nil
	}

	if ctx == nil {
		return errors.New("FLEX settlement context is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopping {
		return errors.New(
			"FLEX settlement controller is stopping",
		)
	}

	if c.started {
		return errors.New(
			"FLEX settlement controller already started",
		)
	}

	if c.lifecycle == nil {
		return errors.New(
			"FLEX settlement lifecycle is missing",
		)
	}

	if err := c.lifecycle.SetOnExit(func(err error) {
		c.mu.Lock()
		stopping := c.stopping
		c.mu.Unlock()

		if !stopping && ctx.Err() == nil &&
			onUnexpectedExit != nil {
			onUnexpectedExit(err)
		}
	}); err != nil {
		return fmt.Errorf(
			"configure FLEX worker supervision: %w",
			err,
		)
	}

	if err := c.lifecycle.Start(ctx, options); err != nil {
		return fmt.Errorf(
			"start FLEX settlement worker: %w",
			err,
		)
	}

	c.started = true
	return nil
}

// StopAndWait requests cancellation and confirms actual exit.
// A timeout is returned to the caller; it is never treated
// as proof that financial processing has stopped.
func (c *flexSettlementController) StopAndWait(
	ctx context.Context,
) error {
	if c == nil {
		return errors.New("FLEX settlement controller is nil")
	}

	if ctx == nil {
		return errors.New(
			"FLEX settlement shutdown context is required",
		)
	}

	if !c.enabled {
		return nil
	}

	c.mu.Lock()
	c.stopping = true
	started := c.started
	lifecycle := c.lifecycle
	c.mu.Unlock()

	if !started {
		return nil
	}

	lifecycle.Stop()

	if err := lifecycle.Wait(ctx); err != nil {
		return fmt.Errorf(
			"wait for FLEX settlement worker shutdown: %w",
			err,
		)
	}

	return nil
}

// Default options are deliberately bounded.
// They do not activate a worker by themselves.
func flexSettlementWorkerOptions(
	onError func(error),
) flexsettlement.DiscoveryWorkerOptions {
	return flexsettlement.DiscoveryWorkerOptions{
		Interval: 5 * time.Second,
		OnError:  onError,
	}
}
