ALTER TABLE jobs ADD COLUMN timezones TEXT[] NOT NULL DEFAULT '{}';
UPDATE jobs SET timezones = ARRAY[timezone] WHERE timezone <> '';
ALTER TABLE jobs ALTER COLUMN timezones DROP DEFAULT;
ALTER TABLE jobs DROP COLUMN timezone;
