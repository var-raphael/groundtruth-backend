CREATE TABLE IF NOT EXISTS card_payments (
	id BIGSERIAL PRIMARY KEY,
	recruiter_id TEXT NOT NULL,
	brand TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS card_payments_brand_idx ON card_payments (brand);
