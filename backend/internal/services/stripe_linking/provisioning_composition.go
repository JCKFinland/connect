package stripelinking

import "errors"

// ErrStripeProvisioningCompositionUnavailable means provisioning
// is disabled or required dependencies are unavailable.
var ErrStripeProvisioningCompositionUnavailable = errors.New(
	"Stripe provisioning composition unavailable",
)

// NewProvisioningCoordinatorIfEnabled composes a coordinator only
// when explicitly enabled with complete trusted dependencies.
//
// It does not construct an SDK client, load credentials,
// contact Stripe, register routes, or authorize payouts.
func NewProvisioningCoordinatorIfEnabled(
	enabled bool,
	deps ProvisioningCoordinatorDeps,
	client StripeAccountCreateClient,
) (*ProvisioningCoordinator, error) {
	if !enabled {
		return nil, ErrStripeProvisioningCompositionUnavailable
	}

	if deps.Permissions == nil ||
		deps.Memberships == nil ||
		deps.Drivers == nil ||
		deps.DB == nil ||
		client == nil ||
		deps.Accounts != nil {
		return nil, ErrStripeProvisioningCompositionUnavailable
	}

	deps.Accounts = NewStripeProvisioningAdapter(
		NewStripeAccountCreateTransport(client),
	)

	return NewProvisioningCoordinator(deps), nil
}
