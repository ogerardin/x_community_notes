-- Add job_id column if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'import_history' AND column_name = 'job_id') THEN
        ALTER TABLE import_history ADD COLUMN job_id UUID DEFAULT gen_random_uuid();
        ALTER TABLE import_history ALTER COLUMN job_id SET NOT NULL;
    END IF;
END $$;

-- Copy any existing data from import_status to import_history (if import_status still exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'import_status') THEN
        INSERT INTO import_history (started_at, completed_at, total_rows, status, error_message, download_percentage, download_speed, download_eta)
        SELECT started_at, completed_at, total_rows, status, error_message, download_percentage, download_speed, download_eta
        FROM import_status WHERE status NOT IN ('idle');

        DROP TABLE IF EXISTS import_status;
    END IF;
END $$;
