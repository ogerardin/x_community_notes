CREATE TABLE IF NOT EXISTS import_history (
    id SERIAL PRIMARY KEY,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    total_rows INT,
    status TEXT CHECK (status IN ('importing', 'completed', 'failed', 'idle', 'downloading')) NOT NULL,
    error_message TEXT,
    download_percentage INT,
    download_speed TEXT,
    download_eta TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_import_history_started_at ON import_history(started_at DESC);
