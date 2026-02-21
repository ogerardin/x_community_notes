package main

import (
	"archive/zip"
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

var (
	port       = "8888"
	dbHost     = getEnv("DB_HOST", "localhost")
	dbPort     = "5432"
	dbUser     = "postgres"
	dbPassword = "example"
	dbName     = "postgres"
	dataDir    = "/home/data"
)

var db *sql.DB
var currentJobID *string

type ImportStatus struct {
	Status             string     `json:"status"`
	TotalRows          *int       `json:"total_rows"`
	PID                *int       `json:"pid,omitempty"`
	RowsProcessed      int        `json:"rows_processed"`
	Percentage         *int       `json:"percentage"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	ErrorMessage       *string    `json:"error_message,omitempty"`
	DownloadPercentage *int       `json:"download_percentage,omitempty"`
	DownloadSpeed      *string    `json:"download_speed,omitempty"`
	FileName           *string    `json:"file_name,omitempty"`
	FileSize           *int64     `json:"file_size,omitempty"`
	ImportDuration     *int       `json:"import_duration,omitempty"`
	TotalFiles         *int       `json:"total_files,omitempty"`
	CurrentFileIndex   *int       `json:"current_file_index,omitempty"`
	FilesProcessed     *int       `json:"files_processed,omitempty"`
}

func initDBWithRetry(maxRetries int, delay time.Duration) error {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	for i := 0; i < maxRetries; i++ {
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			time.Sleep(delay)
			continue
		}

		if err = db.Ping(); err != nil {
			time.Sleep(delay)
			continue
		}

		if err := runMigrations(db); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		return nil
	}
	return fmt.Errorf("failed to connect after %d retries: %w", maxRetries, err)
}

func runMigrations(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file:///migrations",
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Println("Migrations applied successfully")
	return nil
}

func getImportStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	if currentJobID == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ImportStatus{Status: "idle"})
		return
	}

	var status ImportStatus
	var totalRows int
	var startedAt, completedAt sql.NullTime
	var errorMessage sql.NullString
	var downloadPct sql.NullInt64
	var downloadSpeed sql.NullString
	var fileName sql.NullString
	var fileSize sql.NullInt64
	var importDuration sql.NullInt64
	var totalFiles sql.NullInt64
	var currentFileIndex sql.NullInt64
	var filesProcessed sql.NullInt64

	err := db.QueryRowContext(ctx, `
		SELECT status, COALESCE(total_rows, 0), started_at, completed_at, error_message, 
		       COALESCE(download_percentage, 0), download_speed, file_name, file_size, import_duration,
		       COALESCE(total_files, 0), COALESCE(current_file_index, 0), COALESCE(files_processed, 0)
		FROM import_history WHERE job_id = $1
	`, *currentJobID).Scan(&status.Status, &totalRows, &startedAt, &completedAt, &errorMessage, &downloadPct, &downloadSpeed, &fileName, &fileSize, &importDuration, &totalFiles, &currentFileIndex, &filesProcessed)

	if err != nil {
		http.Error(w, "Failed to get import status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	status.TotalRows = &totalRows
	if startedAt.Valid {
		status.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		status.CompletedAt = &completedAt.Time
	}
	if errorMessage.Valid {
		status.ErrorMessage = &errorMessage.String
	}
	if downloadPct.Valid {
		pct := int(downloadPct.Int64)
		status.DownloadPercentage = &pct
	}
	if downloadSpeed.Valid {
		status.DownloadSpeed = &downloadSpeed.String
	}
	if fileName.Valid {
		status.FileName = &fileName.String
	}
	if fileSize.Valid {
		status.FileSize = &fileSize.Int64
	}
	if importDuration.Valid {
		id := int(importDuration.Int64)
		status.ImportDuration = &id
	}
	if totalFiles.Valid {
		tf := int(totalFiles.Int64)
		status.TotalFiles = &tf
	}
	if currentFileIndex.Valid {
		cfi := int(currentFileIndex.Int64)
		status.CurrentFileIndex = &cfi
	}
	if filesProcessed.Valid {
		fp := int(filesProcessed.Int64)
		status.FilesProcessed = &fp
	}

	if status.Status == "importing" {
		var tuplesProcessed int
		err := db.QueryRowContext(ctx, `
			SELECT COALESCE(tuples_processed, 0)
			FROM pg_stat_progress_copy
			LIMIT 1
		`).Scan(&tuplesProcessed)

		if err == nil {
			status.RowsProcessed = tuplesProcessed
			if status.TotalRows != nil && *status.TotalRows > 0 {
				pct := (tuplesProcessed * 100) / *status.TotalRows
				status.Percentage = &pct
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func getImportHistory(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	rows, err := db.QueryContext(ctx, `
		SELECT id, job_id, started_at, completed_at, total_rows, status, error_message, 
		       download_percentage, download_speed, rows_processed, download_cached, download_duration, import_duration, file_name, file_size,
		       total_files, current_file_index, files_processed, file_names
		FROM import_history 
		ORDER BY started_at DESC 
		LIMIT 50
	`)
	if err != nil {
		http.Error(w, "Failed to get import history: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type HistoryEntry struct {
		ID                 int        `json:"id"`
		JobID              string     `json:"job_id"`
		StartedAt          time.Time  `json:"started_at"`
		CompletedAt        *time.Time `json:"completed_at,omitempty"`
		TotalRows          *int       `json:"total_rows,omitempty"`
		Status             string     `json:"status"`
		ErrorMessage       *string    `json:"error_message,omitempty"`
		DownloadPercentage *int       `json:"download_percentage,omitempty"`
		DownloadSpeed      *string    `json:"download_speed,omitempty"`
		RowsProcessed      *int       `json:"rows_processed,omitempty"`
		DownloadCached     *bool      `json:"download_cached,omitempty"`
		DownloadDuration   *int       `json:"download_duration,omitempty"`
		ImportDuration     *int       `json:"import_duration,omitempty"`
		FileName           *string    `json:"file_name,omitempty"`
		FileSize           *int64     `json:"file_size,omitempty"`
		TotalFiles         *int       `json:"total_files,omitempty"`
		CurrentFileIndex   *int       `json:"current_file_index,omitempty"`
		FilesProcessed     *int       `json:"files_processed,omitempty"`
		FileNames          *string    `json:"file_names,omitempty"`
	}

	var history []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		var completedAt sql.NullTime
		var totalRows sql.NullInt64
		var errorMessage sql.NullString
		var downloadPct sql.NullInt64
		var downloadSpeed sql.NullString
		var rowsProcessed sql.NullInt64
		var downloadCached sql.NullBool
		var downloadDuration sql.NullInt64
		var importDuration sql.NullInt64
		var fileName sql.NullString
		var fileSize sql.NullInt64
		var totalFiles sql.NullInt64
		var currentFileIndex sql.NullInt64
		var filesProcessed sql.NullInt64
		var fileNames sql.NullString

		err := rows.Scan(&h.ID, &h.JobID, &h.StartedAt, &completedAt, &totalRows, &h.Status, &errorMessage, &downloadPct, &downloadSpeed, &rowsProcessed, &downloadCached, &downloadDuration, &importDuration, &fileName, &fileSize, &totalFiles, &currentFileIndex, &filesProcessed, &fileNames)
		if err != nil {
			continue
		}

		if completedAt.Valid {
			h.CompletedAt = &completedAt.Time
		}
		if totalRows.Valid {
			rows := int(totalRows.Int64)
			h.TotalRows = &rows
		}
		if errorMessage.Valid {
			h.ErrorMessage = &errorMessage.String
		}
		if downloadPct.Valid {
			pct := int(downloadPct.Int64)
			h.DownloadPercentage = &pct
		}
		if downloadSpeed.Valid {
			h.DownloadSpeed = &downloadSpeed.String
		}
		if rowsProcessed.Valid {
			rp := int(rowsProcessed.Int64)
			h.RowsProcessed = &rp
		}
		if downloadCached.Valid {
			h.DownloadCached = &downloadCached.Bool
		}
		if downloadDuration.Valid {
			dd := int(downloadDuration.Int64)
			h.DownloadDuration = &dd
		}
		if importDuration.Valid {
			id := int(importDuration.Int64)
			h.ImportDuration = &id
		}
		if fileName.Valid {
			h.FileName = &fileName.String
		}
		if fileSize.Valid {
			h.FileSize = &fileSize.Int64
		}
		if totalFiles.Valid {
			tf := int(totalFiles.Int64)
			h.TotalFiles = &tf
		}
		if currentFileIndex.Valid {
			cfi := int(currentFileIndex.Int64)
			h.CurrentFileIndex = &cfi
		}
		if filesProcessed.Valid {
			fp := int(filesProcessed.Int64)
			h.FilesProcessed = &fp
		}
		if fileNames.Valid {
			h.FileNames = &fileNames.String
		}

		history = append(history, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func getDateDaysAgo(n int) string {
	now := time.Now()
	date := now.AddDate(0, 0, -n)
	return date.Format("2006-01-02")
}

func formatDownloadDate(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.Format("2006/01/02")
}

func formatFileDate(date string) string {
	return date + "-notes-00000"
}

func formatFileName(index int) string {
	return fmt.Sprintf("notes-%05d", index)
}

type FileInfo struct {
	ZipPath  string
	TSVPath  string
	FileName string
	FileSize int64
}

func discoverFileCount(ctx context.Context, date string) int {
	baseURL := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/", formatDownloadDate(date))

	for i := 0; i < 100; i++ {
		url := baseURL + fmt.Sprintf("notes-%05d.zip", i)
		req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
		if err != nil {
			return i
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return i
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return i
		}
	}
	return 100
}

type progressTracker struct {
	reader           io.Reader
	totalBytes       int64
	bytesRead        int64
	lastUpdate       time.Time
	lastPct          int
	startTime        time.Time
	ctx              context.Context
	jobID            string
	fileName         string
	totalFiles       int
	currentFileIndex int
}

func (pt *progressTracker) Read(p []byte) (int, error) {
	n, err := pt.reader.Read(p)
	pt.bytesRead += int64(n)

	now := time.Now()
	currentPct := 0
	if pt.totalBytes > 0 {
		currentPct = int((pt.bytesRead * 100) / pt.totalBytes)
	}

	if pt.totalBytes > 0 && (currentPct >= pt.lastPct+5 || now.Sub(pt.lastUpdate) >= time.Second) {
		pt.lastPct = currentPct
		pt.lastUpdate = now

		elapsed := now.Sub(pt.startTime)
		var speedStr string
		if elapsed > 0 {
			bytesPerSec := float64(pt.bytesRead) / elapsed.Seconds()
			speedStr = formatSpeed(bytesPerSec)
		}

		db.ExecContext(pt.ctx,
			`UPDATE import_history SET download_percentage = $1, download_speed = $2, download_duration = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER, file_name = $3, file_size = $4, total_files = $5, current_file_index = $6 WHERE job_id = $7`,
			currentPct, speedStr, pt.fileName, pt.totalBytes, pt.totalFiles, pt.currentFileIndex, pt.jobID)
	}

	return n, err
}

func formatSpeed(bytesPerSec float64) string {
	if bytesPerSec >= 1024*1024 {
		return fmt.Sprintf("(%.1f MB/s)", bytesPerSec/(1024*1024))
	} else if bytesPerSec >= 1024 {
		return fmt.Sprintf("(%.1f KB/s)", bytesPerSec/1024)
	}
	return fmt.Sprintf("(%.0f B/s)", bytesPerSec)
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	} else if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	} else {
		return fmt.Sprintf("%dh %dm", seconds/3600, (seconds%3600)/60)
	}
}

func downloadNotesWithProgress(ctx context.Context, lookbackDays int, jobID string) ([]FileInfo, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	var date string
	for i := 0; i < lookbackDays; i++ {
		date = getDateDaysAgo(i)
		url := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/notes-00000.zip",
			formatDownloadDate(date))

		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			break
		}
	}

	totalFiles := discoverFileCount(ctx, date)
	if totalFiles == 0 {
		return nil, fmt.Errorf("no files found for date %s", date)
	}

	var fileNames []string
	for i := 0; i < totalFiles; i++ {
		fileNames = append(fileNames, fmt.Sprintf("%s-notes-%05d.zip", date, i))
	}
	fileNamesStr := strings.Join(fileNames, ",")

	db.ExecContext(ctx, `UPDATE import_history SET total_files = $1, current_file_index = 0, file_names = $2 WHERE job_id = $3`, totalFiles, fileNamesStr, jobID)

	var files []FileInfo
	for i := 0; i < totalFiles; i++ {
		filename := formatFileName(i) + ".zip"
		filepath := filepath.Join(dataDir, fmt.Sprintf("%s-%s", date, filename))
		url := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/%s",
			formatDownloadDate(date), filename)

		var fileSize int64
		var cached bool

		if _, err := os.Stat(filepath); err == nil {
			fmt.Printf("File already exists: %s\n", filepath)
			info, _ := os.Stat(filepath)
			fileSize = info.Size()
			cached = true

			db.ExecContext(ctx, `UPDATE import_history SET current_file_index = $1, file_name = $2, file_size = $3, download_cached = $4, download_percentage = 100 WHERE job_id = $5`, i, filename, fileSize, cached, jobID)
		} else {
			fmt.Printf("Downloading %s to %s...\n", url, filepath)

			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to download %s: %w", url, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("failed to download %s: status %d", url, resp.StatusCode)
			}

			totalBytes := resp.ContentLength
			tracker := &progressTracker{
				reader:           resp.Body,
				totalBytes:       totalBytes,
				startTime:        time.Now(),
				lastUpdate:       time.Now(),
				ctx:              ctx,
				jobID:            jobID,
				fileName:         filename,
				totalFiles:       totalFiles,
				currentFileIndex: i,
			}

			outFile, err := os.Create(filepath)
			if err != nil {
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, tracker)
			if err != nil {
				os.Remove(filepath)
				return nil, fmt.Errorf("failed to write file: %w", err)
			}

			fileSize = totalBytes
			fmt.Printf("Downloaded %s\n", filepath)
		}

		db.ExecContext(ctx, `UPDATE import_history SET current_file_index = $1, file_name = $2, file_size = $3, download_cached = $4 WHERE job_id = $5`, i, filename, fileSize, cached, jobID)

		tsvPath, err := extractTSV(filepath, i)
		if err != nil {
			return nil, fmt.Errorf("failed to extract %s: %w", filepath, err)
		}

		files = append(files, FileInfo{
			ZipPath:  filepath,
			TSVPath:  tsvPath,
			FileName: filename,
			FileSize: fileSize,
		})
	}

	return files, nil
}

func extractTSV(zipPath string, fileIndex int) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	tsvPath := zipPath[:len(zipPath)-4] + ".tsv"
	expectedTSV := fmt.Sprintf("notes-%05d.tsv", fileIndex)

	for _, file := range reader.File {
		if file.Name == expectedTSV {
			outFile, err := os.Create(tsvPath)
			if err != nil {
				return "", fmt.Errorf("failed to create tsv: %w", err)
			}
			defer outFile.Close()

			rc, err := file.Open()
			if err != nil {
				return "", fmt.Errorf("failed to open zip entry: %w", err)
			}
			defer rc.Close()

			_, err = io.Copy(outFile, rc)
			if err != nil {
				return "", fmt.Errorf("failed to extract tsv: %w", err)
			}

			fmt.Printf("Extracted %s\n", tsvPath)
			return tsvPath, nil
		}
	}

	return "", fmt.Errorf("%s not found in zip", expectedTSV)
}

func countTSVRows(tsvPath string) (int, error) {
	file, err := os.Open(tsvPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	count := 0
	for {
		n, err := file.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if b == '\n' {
					count++
				}
			}
		}
		if err != nil {
			break
		}
	}
	return count - 1, nil
}

func truncateTSV(tsvPath string, maxLines int) error {
	if maxLines <= 0 {
		return nil
	}

	file, err := os.Open(tsvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tmpPath := tsvPath + ".tmp"
	outFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	reader := bufio.NewReader(file)
	for i := 0; i < maxLines; i++ {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if len(line) > 0 {
				outFile.Write(line)
			}
			break
		}
		outFile.Write(line)
	}

	outFile.Close()
	os.Rename(tmpPath, tsvPath)
	return nil
}

func importTSV(ctx context.Context, tsvPath string) error {
	_, err := os.Stat(tsvPath)
	if err != nil {
		return fmt.Errorf("tsv file not found: %w", err)
	}

	_, err = db.ExecContext(ctx, `TRUNCATE note`)
	if err != nil {
		return fmt.Errorf("failed to truncate table: %w", err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf(`COPY note FROM '%s' WITH (FORMAT csv, DELIMITER E'\t', HEADER true)`, tsvPath))
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM note`).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count rows: %w", err)
	}

	_, err = db.ExecContext(ctx, `UPDATE import_status SET status = 'completed', total_rows = $1, completed_at = NOW() WHERE id = 1`, count)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update status: %v\n", err)
	}

	fmt.Printf("Imported %d rows\n", count)
	return nil
}

func triggerImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if currentJobID != nil {
		http.Error(w, "Import already in progress", http.StatusConflict)
		return
	}

	ctx := context.Background()

	limit := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	var jobID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO import_history (started_at, status, download_percentage, rows_processed)
		VALUES (NOW(), 'downloading', 0, 0)
		RETURNING job_id
	`).Scan(&jobID)
	if err != nil {
		http.Error(w, "Failed to create import job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	currentJobID = &jobID

	go func(limit int) {
		ctx := context.Background()

		files, err := downloadNotesWithProgress(ctx, 7, jobID)
		if err != nil {
			db.ExecContext(context.Background(), `UPDATE import_history SET status = 'failed', error_message = $1 WHERE job_id = $2`, err.Error(), jobID)
			currentJobID = nil
			return
		}

		if limit > 0 {
			for _, f := range files {
				fmt.Printf("Truncating %s to %d lines\n", f.TSVPath, limit)
				if err := truncateTSV(f.TSVPath, limit); err != nil {
					fmt.Printf("Warning: failed to truncate %s: %v\n", f.TSVPath, err)
				}
			}
		}

		totalFiles := len(files)
		var totalRows int
		var totalSize int64

		for _, f := range files {
			totalSize += f.FileSize
		}

		var fileNames []string
		for _, f := range files {
			fileNames = append(fileNames, f.FileName)
		}
		fileNamesStr := strings.Join(fileNames, ",")

		db.ExecContext(ctx, `UPDATE import_history SET status = 'importing', download_percentage = 100, total_rows = 0, file_name = $1, file_size = $2, import_started_at = NOW(), files_processed = 0, file_names = $3 WHERE job_id = $4`, fmt.Sprintf("%d files", totalFiles), totalSize, fileNamesStr, jobID)

		_, err = db.ExecContext(ctx, `TRUNCATE note`)
		if err != nil {
			db.ExecContext(context.Background(), `UPDATE import_history SET status = 'failed', error_message = $1 WHERE job_id = $2`, "failed to truncate table: "+err.Error(), jobID)
			currentJobID = nil
			return
		}

		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-done:
					return
				case <-time.After(500 * time.Millisecond):
					var tuplesProcessed int
					err := db.QueryRowContext(context.Background(), `SELECT COALESCE(tuples_processed, 0) FROM pg_stat_progress_copy LIMIT 1`).Scan(&tuplesProcessed)
					if err == nil {
						db.ExecContext(context.Background(), `UPDATE import_history SET rows_processed = $1, import_duration = EXTRACT(EPOCH FROM (NOW() - import_started_at))::INTEGER WHERE job_id = $2`, tuplesProcessed, jobID)
					}
				}
			}
		}()

		for i, f := range files {
			db.ExecContext(ctx, `UPDATE import_history SET current_file_index = $1, file_name = $2 WHERE job_id = $3`, i, f.FileName, jobID)

			_, err = db.ExecContext(ctx, fmt.Sprintf(`COPY note FROM '%s' WITH (FORMAT csv, DELIMITER E'\t', HEADER true)`, f.TSVPath))
			if err != nil {
				close(done)
				db.ExecContext(context.Background(), `UPDATE import_history SET status = 'failed', error_message = $1, completed_at = NOW() WHERE job_id = $2`, "failed to import "+f.FileName+": "+err.Error(), jobID)
				currentJobID = nil
				return
			}

			var count int
			err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM note`).Scan(&count)
			if err == nil {
				totalRows = count
			}

			db.ExecContext(ctx, `UPDATE import_history SET files_processed = $1, total_rows = $2 WHERE job_id = $3`, i+1, totalRows, jobID)
			fmt.Printf("Imported %s (%d of %d)\n", f.FileName, i+1, totalFiles)
		}

		close(done)

		var importDuration int
		err = db.QueryRowContext(ctx, `SELECT EXTRACT(EPOCH FROM (NOW() - import_started_at))::INTEGER FROM import_history WHERE job_id = $1`, jobID).Scan(&importDuration)
		if err != nil {
			importDuration = 0
		}

		_, err = db.ExecContext(ctx, `UPDATE import_history SET status = 'completed', total_rows = $1, completed_at = NOW(), import_duration = $2 WHERE job_id = $3`, totalRows, importDuration, jobID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update status: %v\n", err)
		}

		currentJobID = nil
		fmt.Printf("Import completed: %d rows from %d files\n", totalRows, totalFiles)
	}(limit)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "Import started"})
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func sanitizeImportStatus() {
	ctx := context.Background()
	currentJobID = nil

	_, err := db.ExecContext(ctx, `
		UPDATE import_history 
		SET status = 'failed', error_message = 'Interrupted'
		WHERE status IN ('importing', 'downloading')
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to sanitize import status: %v\n", err)
		return
	}

	fmt.Println("Cleared any running import jobs")
}

func main() {
	if err := initDBWithRetry(30, time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	sanitizeImportStatus()

	http.HandleFunc("/health", healthCheck)
	http.HandleFunc("/import/status", getImportStatus)
	http.HandleFunc("/import/history", getImportHistory)
	http.HandleFunc("/import/trigger", triggerImport)

	fmt.Printf("Starting API server on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
		os.Exit(1)
	}
}
