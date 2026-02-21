ALTER TABLE import_history DROP COLUMN IF EXISTS total_files;
ALTER TABLE import_history DROP COLUMN IF EXISTS current_file_index;
ALTER TABLE import_history DROP COLUMN IF EXISTS files_processed;
