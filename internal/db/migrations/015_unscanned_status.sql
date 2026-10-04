ALTER TABLE candidates DROP CONSTRAINT status_valid;
ALTER TABLE candidates ADD CONSTRAINT status_valid CHECK (status IN ('queued', 'extracting', 'extracted', 'scoring', 'scored', 'failed', 'unscanned'));
