CREATE TABLE IF NOT EXISTS import_status (
    id SERIAL PRIMARY KEY,
    started_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    total_rows INT,
    pid INT,
    status TEXT CHECK (status IN ('running', 'completed', 'failed', 'idle', 'downloading')) DEFAULT 'idle',
    error_message TEXT,
    download_percentage INT,
    download_speed TEXT,
    download_eta TEXT
);

INSERT INTO import_status (id, status) VALUES (1, 'idle') 
ON CONFLICT (id) DO NOTHING;
