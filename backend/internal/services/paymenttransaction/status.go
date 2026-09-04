package paymenttransaction

const (
	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusSuccess    = "SUCCESS"
	StatusFailed     = "FAILED"
	StatusCancelled  = "CANCELLED"
)

func isValidStatus(
	status string,
) bool {
	switch status {
	case StatusPending,
		StatusProcessing,
		StatusSuccess,
		StatusFailed,
		StatusCancelled:

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
			StatusSuccess,
			StatusFailed,
			StatusCancelled:

			return true
		}

	case StatusProcessing:
		switch to {
		case StatusSuccess,
			StatusFailed,
			StatusCancelled:

			return true
		}

	case StatusSuccess,
		StatusFailed,
		StatusCancelled:

		return false
	}

	return false
}
