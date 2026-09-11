ALTER TABLE candidates ADD COLUMN status_updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
