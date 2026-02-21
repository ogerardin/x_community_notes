ALTER TABLE import_history ADD COLUMN IF NOT EXISTS total_files INT;
ALTER TABLE import_history ADD COLUMN IF NOT EXISTS current_file_index INT;
ALTER TABLE import_history ADD COLUMN IF NOT EXISTS files_processed INT;
