package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TripEventRepository implements repository.TripEventRepository.
type TripEventRepository struct {
	db DBTX
}

// Compile-time interface check.
var _ repository.TripEventRepository = (*TripEventRepository)(nil)

// NewTripEventRepository creates a PostgreSQL trip event repository.
func NewTripEventRepository(
	db *pgxpool.Pool,
) *TripEventRepository {
	return &TripEventRepository{
		db: db,
	}
}

// NewTripEventRepositoryWithDB creates a trip event repository using
// either the connection pool or an active transaction.
func NewTripEventRepositoryWithDB(
	db DBTX,
) *TripEventRepository {
	return &TripEventRepository{
		db: db,
	}
}

// Create stores an immutable trip event.
func (r *TripEventRepository) Create(
	ctx context.Context,
	event *models.TripEvent,
) error {

	if event == nil {
		return fmt.Errorf("trip event is required")
	}

	if event.ID == "" {
		event.ID = uuid.NewString()
	}

	const query = `
		INSERT INTO trip_events
		(
			id,
			trip_id,
			event_type,
			performed_by_user_id,
			latitude,
			longitude,
			metadata,
			occurred_at
		)
		VALUES
		(
			$1,$2,$3,$4,$5,$6,$7,$8
		)
		RETURNING occurred_at
	`

	return r.db.QueryRow(
		ctx,
		query,
		event.ID,
		event.TripID,
		event.EventType,
		event.PerformedByUserID,
		event.Latitude,
		event.Longitude,
		event.Metadata,
		event.OccurredAt,
	).Scan(
		&event.OccurredAt,
	)
}

// ListByTripID returns all trip events in chronological order.
func (r *TripEventRepository) ListByTripID(
	ctx context.Context,
	tripID string,
) ([]*models.TripEvent, error) {

	const query = `
		SELECT
			id,
			trip_id,
			event_type,
			performed_by_user_id,
			latitude,
			longitude,
			metadata,
			occurred_at
		FROM trip_events
		WHERE trip_id = $1
		ORDER BY occurred_at ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list trip events: %w",
			err,
		)
	}
	defer rows.Close()

	events := make([]*models.TripEvent, 0)

	for rows.Next() {
		event := &models.TripEvent{}

		if err := rows.Scan(
			&event.ID,
			&event.TripID,
			&event.EventType,
			&event.PerformedByUserID,
			&event.Latitude,
			&event.Longitude,
			&event.Metadata,
			&event.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf(
				"scan trip event: %w",
				err,
			)
		}

		events = append(
			events,
			event,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate trip events: %w",
			err,
		)
	}

	return events, nil
}
