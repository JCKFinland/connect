package driver_document

import "errors"

var (
	// ErrInvalidDocument indicates that the supplied driver-document
	// metadata does not satisfy CONNECT's document requirements.
	ErrInvalidDocument = errors.New(
		"invalid driver document",
	)

	// ErrDriverNotFound indicates that the target operational driver
	// does not exist or is not available for document submission.
	ErrDriverNotFound = errors.New(
		"driver not found",
	)

	// ErrDocumentNotFound indicates that the requested active driver
	// document does not exist.
	ErrDocumentNotFound = errors.New(
		"driver document not found",
	)

	// ErrDocumentAlreadyReviewed indicates that a regulatory review
	// decision has already resolved the document.
	ErrDocumentAlreadyReviewed = errors.New(
		"driver document has already been reviewed",
	)
)
