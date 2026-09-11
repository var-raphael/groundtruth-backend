package queries

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Recruiter struct {
	ID        string
	GoogleID  string
	Email     string
	Name      string
	Plan      string
	CreatedAt time.Time
}

func UpsertRecruiterFromGoogle(ctx context.Context, pool *pgxpool.Pool, googleID, email, name string) (*Recruiter, error) {
	const q = `
		INSERT INTO recruiters (google_id, email, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (google_id) DO UPDATE SET
			email = EXCLUDED.email,
			name = EXCLUDED.name
		RETURNING id, google_id, email, name, plan, created_at`

	var r Recruiter
	err := pool.QueryRow(ctx, q, googleID, email, name).Scan(
		&r.ID, &r.GoogleID, &r.Email, &r.Name, &r.Plan, &r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upserting recruiter from google sign-in: %w", err)
	}
	return &r, nil
}

func GetRecruiterByID(ctx context.Context, pool *pgxpool.Pool, id string) (*Recruiter, error) {
	const q = `SELECT id, google_id, email, name, plan, created_at FROM recruiters WHERE id = $1`

	var r Recruiter
	err := pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.GoogleID, &r.Email, &r.Name, &r.Plan, &r.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching recruiter %s: %w", id, err)
	}
	return &r, nil
}

func GetRecruiterByGoogleID(ctx context.Context, pool *pgxpool.Pool, googleID string) (*Recruiter, error) {
	const q = `SELECT id, google_id, email, name, plan, created_at FROM recruiters WHERE google_id = $1`

	var r Recruiter
	err := pool.QueryRow(ctx, q, googleID).Scan(
		&r.ID, &r.GoogleID, &r.Email, &r.Name, &r.Plan, &r.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching recruiter by google_id: %w", err)
	}
	return &r, nil
}
