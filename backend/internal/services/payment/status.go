package payment

const (
	StatusPending           = "PENDING"
	StatusProcessing        = "PROCESSING"
	StatusAuthorized        = "AUTHORIZED"
	StatusPaid              = "PAID"
	StatusFailed            = "FAILED"
	StatusCancelled         = "CANCELLED"
	StatusRefunded          = "REFUNDED"
	StatusPartiallyRefunded = "PARTIALLY_REFUNDED"
)

func isValidStatus(
	status string,
) bool {
	switch status {
	case StatusPending,
		StatusProcessing,
		StatusAuthorized,
		StatusPaid,
		StatusFailed,
		StatusCancelled,
		StatusRefunded,
		StatusPartiallyRefunded:

		return true

	default:
		return false
	}
}

func canTransition(
	from string,
	to string,
) bool {
	switch from {
	case StatusPending:
		switch to {
		case StatusProcessing,
			StatusAuthorized,
			StatusCancelled,
			StatusPaid:

			return true
		}

	case StatusProcessing:
		switch to {
		case StatusAuthorized,
			StatusPaid,
			StatusFailed,
			StatusCancelled:

			return true
		}

	case StatusAuthorized:
		switch to {
		case StatusPaid,
			StatusCancelled:

			return true
		}

	case StatusPaid:
		switch to {
		case StatusPartiallyRefunded,
			StatusRefunded:

			return true
		}

	case StatusPartiallyRefunded:
		switch to {
		case StatusPartiallyRefunded,
			StatusRefunded:

			return true
		}

	case StatusFailed,
		StatusCancelled,
		StatusRefunded:

		return false
	}

	return false
}

// CanTransitionStatus exposes the authoritative payment lifecycle
// transition rule to financial reconciliation services.
func CanTransitionStatus(
	from string,
	to string,
) bool {
	return canTransition(
		from,
		to,
	)
}
