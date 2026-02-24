CREATE TABLE IF NOT EXISTS import_history (
    id SERIAL PRIMARY KEY,
    job_id UUID DEFAULT gen_random_uuid() NOT NULL,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    total_rows INT,
    status TEXT CHECK (status IN ('importing', 'completed', 'failed', 'idle', 'downloading')) NOT NULL,
    error_message TEXT,
    download_percentage INT,
    download_speed TEXT,
    download_eta TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    rows_processed INT,
    download_completed_at TIMESTAMP,
    download_cached BOOLEAN DEFAULT false,
    download_duration INT,
    import_started_at TIMESTAMP,
    import_duration INT,
    file_name TEXT,
    file_size BIGINT,
    total_files INT,
    current_file_index INT,
    files_processed INT,
    file_names TEXT
);

CREATE INDEX IF NOT EXISTS idx_import_history_started_at ON import_history(started_at DESC);
