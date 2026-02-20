ALTER TABLE import_history ADD COLUMN IF NOT EXISTS import_started_at TIMESTAMP;
ALTER TABLE import_history ADD COLUMN IF NOT EXISTS import_duration INTEGER;
