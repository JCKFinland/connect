package postgres

import (
	"context"
	"fmt"
	"time"
)

// RegulatoryDriverCandidate identifies an online idle driver whose
// regulatory state requires authoritative compliance re-evaluation.
type RegulatoryDriverCandidate struct {
	DriverID string
	UserID   string
}

// ListRegulatoryCandidates discovers potentially non-compliant online idle
// drivers without taking locks. The caller must lock the permanent driver
// aggregate and re-evaluate compliance before changing operational presence.
//
// Expiration uses UTC calendar dates, matching driver_compliance.Evaluate.
// afterDriverID is an exclusive keyset cursor.
func (r *DriverPresenceRepository) ListRegulatoryCandidates(
	ctx context.Context,
	now time.Time,
	afterDriverID string,
	limit int,
) ([]RegulatoryDriverCandidate, error) {
	if now.IsZero() {
		return nil, fmt.Errorf("reconciliation time is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("candidate limit must be greater than zero")
	}

	const query = `
		SELECT d.id, d.user_id
		FROM driver_presence AS dp
		JOIN drivers AS d ON d.user_id = dp.driver_id
		LEFT JOIN driver_documents AS dl
			ON dl.driver_id = d.id
			AND dl.document_type = 'DRIVING_LICENSE'
			AND dl.deleted_at IS NULL
		LEFT JOIN driver_documents AS tl
			ON tl.driver_id = d.id
			AND tl.document_type = 'TAXI_DRIVER_LICENSE'
			AND tl.deleted_at IS NULL
		WHERE d.deleted_at IS NULL
		  AND dp.is_online = TRUE
		  AND dp.availability_status IN ('AVAILABLE', 'BREAK')
		  AND (
			d.is_active = FALSE
			OR d.is_verified = FALSE
			OR d.status <> 'ACTIVE'
			OR d.driving_license_expiry IS NULL
			OR d.driving_license_expiry < $1::date
			OR dl.id IS NULL
			OR dl.status <> 'VERIFIED'
			OR dl.expires_at IS NULL
			OR dl.expires_at < $1::date
			OR tl.id IS NULL
			OR tl.status <> 'VERIFIED'
			OR tl.expires_at IS NULL
			OR tl.expires_at < $1::date
		  )
		  AND ($2::uuid IS NULL OR d.id > $2::uuid)
		ORDER BY d.id ASC
		LIMIT $3
	`

	var cursor any
	if afterDriverID != "" {
		cursor = afterDriverID
	}

	rows, err := r.db.Query(
		ctx,
		query,
		now.UTC().Format("2006-01-02"),
		cursor,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list regulatory driver candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]RegulatoryDriverCandidate, 0, limit)
	for rows.Next() {
		var candidate RegulatoryDriverCandidate
		if err := rows.Scan(&candidate.DriverID, &candidate.UserID); err != nil {
			return nil, fmt.Errorf("scan regulatory driver candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate regulatory driver candidates: %w", err)
	}

	return candidates, nil
}
