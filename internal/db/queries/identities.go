package queries

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/secrets"
)

type CandidateIdentity struct {
	AuthUserID     string
	GithubID       int64
	GithubUsername string
	GithubToken    string
}

func UpsertCandidateIdentity(ctx context.Context, pool *pgxpool.Pool, id CandidateIdentity) error {
	encrypted, err := secrets.Encrypt(id.GithubToken)
	if err != nil {
		return fmt.Errorf("encrypting identity token: %w", err)
	}

	const q = `
		INSERT INTO candidate_identities (auth_user_id, github_id, github_username, github_token)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (auth_user_id) DO UPDATE
		SET github_id = EXCLUDED.github_id,
			github_username = EXCLUDED.github_username,
			github_token = EXCLUDED.github_token,
			updated_at = now()`

	if _, err := pool.Exec(ctx, q, id.AuthUserID, id.GithubID, id.GithubUsername, encrypted); err != nil {
		return fmt.Errorf("saving candidate identity: %w", err)
	}
	return nil
}

func GetCandidateIdentity(ctx context.Context, pool *pgxpool.Pool, authUserID string) (*CandidateIdentity, error) {
	const q = `
		SELECT auth_user_id, github_id, github_username, github_token
		FROM candidate_identities WHERE auth_user_id = $1`

	var id CandidateIdentity
	var stored string
	err := pool.QueryRow(ctx, q, authUserID).Scan(&id.AuthUserID, &id.GithubID, &id.GithubUsername, &stored)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching candidate identity: %w", err)
	}

	plain, err := secrets.Decrypt(stored)
	if err != nil {
		return nil, fmt.Errorf("decrypting identity token: %w", err)
	}
	id.GithubToken = plain
	return &id, nil
}
