ALTER TABLE recruiters
    ADD COLUMN paystack_customer_code     TEXT,
    ADD COLUMN paystack_subscription_code TEXT,
    ADD COLUMN paystack_email_token       TEXT,
    ADD COLUMN plan_expires_at            TIMESTAMPTZ;

CREATE INDEX idx_recruiters_paystack_customer ON recruiters(paystack_customer_code);
CREATE INDEX idx_recruiters_paystack_subscription ON recruiters(paystack_subscription_code);

CREATE TABLE payment_events (
    event_hash TEXT PRIMARY KEY,
    event      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
