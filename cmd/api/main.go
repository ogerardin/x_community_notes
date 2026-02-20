package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

	err := db.QueryRowContext(ctx, `
		SELECT status, COALESCE(total_rows, 0), started_at, completed_at, error_message, 
		       COALESCE(download_percentage, 0), download_speed, file_name, file_size, import_duration
		FROM import_history WHERE job_id = $1
	`, *currentJobID).Scan(&status.Status, &totalRows, &startedAt, &completedAt, &errorMessage, &downloadPct, &downloadSpeed, &fileName, &fileSize, &importDuration)

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
		       download_percentage, download_speed, rows_processed, download_cached, download_duration, import_duration, file_name, file_size
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

		err := rows.Scan(&h.ID, &h.JobID, &h.StartedAt, &completedAt, &totalRows, &h.Status, &errorMessage, &downloadPct, &downloadSpeed, &rowsProcessed, &downloadCached, &downloadDuration, &importDuration, &fileName, &fileSize)
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

type progressTracker struct {
	reader     io.Reader
	totalBytes int64
	bytesRead  int64
	lastUpdate time.Time
	lastPct    int
	startTime  time.Time
	ctx        context.Context
	jobID      string
	fileName   string
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
			`UPDATE import_history SET download_percentage = $1, download_speed = $2, download_duration = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER, file_name = $3, file_size = $4 WHERE job_id = $5`,
			currentPct, speedStr, pt.fileName, pt.totalBytes, pt.jobID)
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

func downloadNotesWithProgress(ctx context.Context, lookbackDays int, jobID string) (string, int64, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create data directory: %w", err)
	}

	for i := 0; i < lookbackDays; i++ {
		date := getDateDaysAgo(i)
		filename := formatFileDate(date) + ".zip"
		filepath := filepath.Join(dataDir, filename)
		url := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/notes-00000.zip",
			formatDownloadDate(date))

		if _, err := os.Stat(filepath); err == nil {
			fmt.Printf("File already exists: %s\n", filepath)
			info, _ := os.Stat(filepath)
			return filepath, info.Size(), nil
		}

		fmt.Printf("Downloading %s to %s...\n", url, filepath)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			continue
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("Failed to download %s: %v\n", url, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Failed to download %s: status %d\n", url, resp.StatusCode)
			continue
		}

		totalBytes := resp.ContentLength
		tracker := &progressTracker{
			reader:     resp.Body,
			totalBytes: totalBytes,
			startTime:  time.Now(),
			lastUpdate: time.Now(),
			ctx:        ctx,
			jobID:      jobID,
			fileName:   filename,
		}

		outFile, err := os.Create(filepath)
		if err != nil {
			return "", 0, fmt.Errorf("failed to create file: %w", err)
		}
		defer outFile.Close()

		_, err = io.Copy(outFile, tracker)
		if err != nil {
			os.Remove(filepath)
			return "", 0, fmt.Errorf("failed to write file: %w", err)
		}

		fmt.Printf("Downloaded %s\n", filepath)
		return filepath, totalBytes, nil
	}

	return "", 0, fmt.Errorf("failed to download notes file for last %d days", lookbackDays)
}

func extractTSV(zipPath string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	tsvPath := zipPath[:len(zipPath)-4] + ".tsv"

	for _, file := range reader.File {
		if file.Name == "notes-00000.tsv" {
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

	return "", fmt.Errorf("notes-00000.tsv not found in zip")
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

	go func() {
		ctx := context.Background()

		zipPath, fileSize, err := downloadNotesWithProgress(ctx, 7, jobID)
		if err != nil {
			db.ExecContext(context.Background(), `UPDATE import_history SET status = 'failed', error_message = $1 WHERE job_id = $2`, err.Error(), jobID)
			currentJobID = nil
			return
		}

		fileName := filepath.Base(zipPath)

		tsvPath, err := extractTSV(zipPath)
		if err != nil {
			db.ExecContext(context.Background(), `UPDATE import_history SET status = 'failed', error_message = $1 WHERE job_id = $2`, err.Error(), jobID)
			currentJobID = nil
			return
		}

		totalRows, err := countTSVRows(tsvPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to count rows: %v\n", err)
			totalRows = 0
		}

		db.ExecContext(ctx, `UPDATE import_history SET status = 'importing', download_percentage = 100, total_rows = $1, file_name = $2, file_size = $3, import_started_at = NOW() WHERE job_id = $4`, totalRows, fileName, fileSize, jobID)

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

		err = importTSV(ctx, tsvPath)
		close(done)
		if err != nil {
			db.ExecContext(context.Background(), `UPDATE import_history SET status = 'failed', error_message = $1, completed_at = NOW() WHERE job_id = $2`, err.Error(), jobID)
			currentJobID = nil
			return
		}

		var count int
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM note`).Scan(&count)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to count rows: %v\n", err)
		}

		_, err = db.ExecContext(ctx, `UPDATE import_history SET status = 'completed', total_rows = $1, completed_at = NOW() WHERE job_id = $2`, count, jobID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update status: %v\n", err)
		}

		currentJobID = nil
		fmt.Printf("Imported %d rows\n", count)
	}()

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
