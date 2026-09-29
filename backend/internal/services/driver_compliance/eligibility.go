package driver_compliance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Eligibility describes whether a driver currently satisfies the regulatory
// requirements required to participate in operational dispatch.
type Eligibility struct {
	Eligible bool
	Reasons  []string
}

const (
	ReasonDriverInactive            = "DRIVER_INACTIVE"
	ReasonDriverNotVerified         = "DRIVER_NOT_VERIFIED"
	ReasonDriverStatusNotActive     = "DRIVER_STATUS_NOT_ACTIVE"
	ReasonDrivingLicenseExpired     = "DRIVING_LICENSE_EXPIRED"
	ReasonDrivingLicenseMissing     = "DRIVING_LICENSE_DOCUMENT_MISSING"
	ReasonDrivingLicenseNotVerified = "DRIVING_LICENSE_DOCUMENT_NOT_VERIFIED"
	ReasonDrivingLicenseDocExpired  = "DRIVING_LICENSE_DOCUMENT_EXPIRED"
	ReasonTaxiLicenseMissing        = "TAXI_DRIVER_LICENSE_DOCUMENT_MISSING"
	ReasonTaxiLicenseNotVerified    = "TAXI_DRIVER_LICENSE_DOCUMENT_NOT_VERIFIED"
	ReasonTaxiLicenseExpired        = "TAXI_DRIVER_LICENSE_DOCUMENT_EXPIRED"
)

// Evaluate determines whether the specified operational driver is currently
// eligible to go online and participate in dispatch.
//
// Missing or non-compliant regulatory state produces an ineligible result.
// Infrastructure failures are returned as errors so callers do not mistake
// an unavailable compliance check for a valid eligibility decision.
func (s *Service) Evaluate(
	ctx context.Context,
	driverID string,
	now time.Time,
) (*Eligibility, error) {
	if s == nil {
		return nil, errors.New("driver compliance service is required")
	}

	if s.drivers == nil {
		return nil, errors.New("driver repository is required")
	}

	if s.documents == nil {
		return nil, errors.New("driver document repository is required")
	}

	driverID = strings.TrimSpace(driverID)
	if driverID == "" {
		return nil, errors.New("driver ID is required")
	}

	now = now.UTC()

	driver, err := s.drivers.GetByID(ctx, driverID)
	if errors.Is(err, repository.ErrNotFound) {
		return &Eligibility{
			Eligible: false,
			Reasons:  []string{ReasonDriverInactive},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get driver for compliance evaluation: %w", err)
	}

	if driver == nil {
		return &Eligibility{
			Eligible: false,
			Reasons:  []string{ReasonDriverInactive},
		}, nil
	}

	result := &Eligibility{
		Eligible: true,
		Reasons:  make([]string, 0, 6),
	}

	if !driver.IsActive {
		result.addReason(ReasonDriverInactive)
	}

	if !driver.IsVerified {
		result.addReason(ReasonDriverNotVerified)
	}

	if driver.Status != "ACTIVE" {
		result.addReason(ReasonDriverStatusNotActive)
	}

	if driver.DrivingLicenseExpiry == nil ||
		!driver.DrivingLicenseExpiry.After(now) {

		result.addReason(ReasonDrivingLicenseExpired)
	}

	if err := s.evaluateDocument(
		ctx,
		driver.ID,
		models.DriverDocumentTypeDrivingLicense,
		now,
		result,
		ReasonDrivingLicenseMissing,
		ReasonDrivingLicenseNotVerified,
		ReasonDrivingLicenseDocExpired,
	); err != nil {
		return nil, err
	}

	if err := s.evaluateDocument(
		ctx,
		driver.ID,
		models.DriverDocumentTypeTaxiDriverLicense,
		now,
		result,
		ReasonTaxiLicenseMissing,
		ReasonTaxiLicenseNotVerified,
		ReasonTaxiLicenseExpired,
	); err != nil {
		return nil, err
	}

	result.Eligible = len(result.Reasons) == 0

	return result, nil
}

func (s *Service) evaluateDocument(
	ctx context.Context,
	driverID string,
	documentType string,
	now time.Time,
	result *Eligibility,
	missingReason string,
	notVerifiedReason string,
	expiredReason string,
) error {
	document, err := s.documents.GetByDriverAndType(
		ctx,
		driverID,
		documentType,
	)

	if errors.Is(err, repository.ErrNotFound) {
		result.addReason(missingReason)
		return nil
	}

	if err != nil {
		return fmt.Errorf(
			"get %s for compliance evaluation: %w",
			documentType,
			err,
		)
	}

	if document == nil {
		result.addReason(missingReason)
		return nil
	}

	if document.Status != models.DriverDocumentStatusVerified {
		result.addReason(notVerifiedReason)
	}

	if document.ExpiresAt == nil ||
		!document.ExpiresAt.After(now) {

		result.addReason(expiredReason)
	}

	return nil
}

func (e *Eligibility) addReason(reason string) {
	if e == nil {
		return
	}

	e.Eligible = false
	e.Reasons = append(e.Reasons, reason)
}
