package queries

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/secrets"
)

type Billing struct {
	RecruiterID      string
	Email            string
	Plan             string
	CustomerCode     string
	SubscriptionCode string
	EmailToken       string
	PlanExpiresAt    *time.Time
}

func GetBilling(ctx context.Context, pool *pgxpool.Pool, recruiterID string) (*Billing, error) {
	const q = `
		SELECT id, email, plan,
			COALESCE(paystack_customer_code, ''),
			COALESCE(paystack_subscription_code, ''),
			COALESCE(paystack_email_token, ''),
			plan_expires_at
		FROM recruiters WHERE id = $1`

	var b Billing
	err := pool.QueryRow(ctx, q, recruiterID).Scan(
		&b.RecruiterID, &b.Email, &b.Plan, &b.CustomerCode, &b.SubscriptionCode, &b.EmailToken, &b.PlanExpiresAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching billing for recruiter %s: %w", recruiterID, err)
	}

	if b.EmailToken != "" {
		plain, err := secrets.Decrypt(b.EmailToken)
		if err != nil {
			b.EmailToken = ""
		} else {
			b.EmailToken = plain
		}
	}
	return &b, nil
}

func FindRecruiterForPayment(ctx context.Context, pool *pgxpool.Pool, recruiterID, subscriptionCode, customerCode, email string) (string, error) {
	lookups := []struct {
		query string
		value string
	}{
		{`SELECT id FROM recruiters WHERE id::text = $1`, recruiterID},
		{`SELECT id FROM recruiters WHERE paystack_subscription_code = $1`, subscriptionCode},
		{`SELECT id FROM recruiters WHERE paystack_customer_code = $1`, customerCode},
		{`SELECT id FROM recruiters WHERE lower(email) = lower($1)`, email},
	}

	for _, l := range lookups {
		if l.value == "" {
			continue
		}
		var id string
		err := pool.QueryRow(ctx, l.query, l.value).Scan(&id)
		if err == pgx.ErrNoRows {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("looking up recruiter for payment: %w", err)
		}
		return id, nil
	}
	return "", nil
}

func ActivateProPlan(ctx context.Context, pool *pgxpool.Pool, recruiterID, customerCode string) error {
	const q = `
		UPDATE recruiters
		SET plan = CASE WHEN plan = 'internal' THEN plan ELSE 'pro' END,
			paystack_customer_code = COALESCE(NULLIF($2, ''), paystack_customer_code),
			plan_expires_at = NULL
		WHERE id = $1`
	if _, err := pool.Exec(ctx, q, recruiterID, customerCode); err != nil {
		return fmt.Errorf("activating pro plan: %w", err)
	}
	return nil
}

func SaveSubscription(ctx context.Context, pool *pgxpool.Pool, recruiterID, subscriptionCode, emailToken, customerCode string) error {
	storedToken := ""
	if emailToken != "" {
		enc, err := secrets.Encrypt(emailToken)
		if err != nil {
			return fmt.Errorf("encrypting subscription token: %w", err)
		}
		storedToken = enc
	}

	const q = `
		UPDATE recruiters
		SET paystack_subscription_code = $2,
			paystack_email_token = COALESCE(NULLIF($3, ''), paystack_email_token),
			paystack_customer_code = COALESCE(NULLIF($4, ''), paystack_customer_code)
		WHERE id = $1`
	if _, err := pool.Exec(ctx, q, recruiterID, subscriptionCode, storedToken, customerCode); err != nil {
		return fmt.Errorf("saving subscription: %w", err)
	}
	return nil
}

func SetPlanExpiry(ctx context.Context, pool *pgxpool.Pool, recruiterID string, expiresAt time.Time) error {
	const q = `UPDATE recruiters SET plan_expires_at = $2 WHERE id = $1 AND plan = 'pro'`
	if _, err := pool.Exec(ctx, q, recruiterID, expiresAt); err != nil {
		return fmt.Errorf("setting plan expiry: %w", err)
	}
	return nil
}

func DowngradeToFree(ctx context.Context, pool *pgxpool.Pool, recruiterID string) error {
	const q = `
		UPDATE recruiters
		SET plan = 'free', plan_expires_at = NULL,
			paystack_subscription_code = NULL, paystack_email_token = NULL
		WHERE id = $1 AND plan = 'pro'`
	if _, err := pool.Exec(ctx, q, recruiterID); err != nil {
		return fmt.Errorf("downgrading recruiter: %w", err)
	}
	return nil
}

func ExpireLapsedPlans(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	const q = `
		UPDATE recruiters
		SET plan = 'free', plan_expires_at = NULL,
			paystack_subscription_code = NULL, paystack_email_token = NULL
		WHERE plan = 'pro' AND plan_expires_at IS NOT NULL AND plan_expires_at < now()`
	tag, err := pool.Exec(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("expiring lapsed plans: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func PaymentEventSeen(ctx context.Context, pool *pgxpool.Pool, hash string) (bool, error) {
	var seen bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_events WHERE event_hash = $1)`, hash).Scan(&seen)
	if err != nil {
		return false, fmt.Errorf("checking payment event: %w", err)
	}
	return seen, nil
}

func MarkPaymentEvent(ctx context.Context, pool *pgxpool.Pool, hash, event string) error {
	const q = `INSERT INTO payment_events (event_hash, event) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	if _, err := pool.Exec(ctx, q, hash, event); err != nil {
		return fmt.Errorf("recording payment event: %w", err)
	}
	return nil
}

func ExpireStalePayments(ctx context.Context, pool *pgxpool.Pool, days int) (int, error) {
	const q = `
		UPDATE recruiters
		SET plan = 'free', plan_expires_at = NULL,
			paystack_subscription_code = NULL, paystack_email_token = NULL
		WHERE plan = 'pro'
			AND plan_expires_at IS NULL
			AND id::text IN (
				SELECT recruiter_id FROM card_payments
				GROUP BY recruiter_id
				HAVING max(created_at) < now() - make_interval(days => $1)
			)`
	tag, err := pool.Exec(ctx, q, days)
	if err != nil {
		return 0, fmt.Errorf("expiring stale payments: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func RecordCardBrand(ctx context.Context, pool *pgxpool.Pool, recruiterID, brand string) error {
	const q = `INSERT INTO card_payments (recruiter_id, brand) VALUES ($1, $2)`
	if _, err := pool.Exec(ctx, q, recruiterID, brand); err != nil {
		return fmt.Errorf("recording card brand: %w", err)
	}
	return nil
}
