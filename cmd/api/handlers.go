package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func isImportAborted(jobID string) bool {
	var status string
	err := db.QueryRowContext(context.Background(), `SELECT status FROM import_history WHERE job_id = $1`, jobID).Scan(&status)
	if err != nil {
		return false
	}
	return status == "failed"
}

func getImportByID(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	jobID := r.PathValue("job_id")

	if jobID == "" {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "Job ID is required")
		return
	}

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
	var fileSize sql.NullInt64
	var totalFiles sql.NullInt64
	var currentFileIndex sql.NullInt64
	var filesProcessed sql.NullInt64
	var fileNames sql.NullString

	err := db.QueryRowContext(ctx, `
		SELECT id, job_id, started_at, completed_at, total_rows, status, error_message, 
		       download_percentage, download_speed, rows_processed, download_cached, download_duration, import_duration, file_size,
		       total_files, current_file_index, files_processed, file_names
		FROM import_history 
		WHERE job_id = $1
	`, jobID).Scan(&h.ID, &h.JobID, &h.StartedAt, &completedAt, &totalRows, &h.Status, &errorMessage, &downloadPct, &downloadSpeed, &rowsProcessed, &downloadCached, &downloadDuration, &importDuration, &fileSize, &totalFiles, &currentFileIndex, &filesProcessed, &fileNames)

	if err == sql.ErrNoRows {
		writeProblem(w, http.StatusNotFound, "Not Found", "Import job not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "Failed to get import: "+err.Error())
		return
	}

	h.CompletedAt = nullTimeToTimePtr(completedAt)
	h.TotalRows = nullInt64ToIntPtr(totalRows)
	h.ErrorMessage = nullStringToStrPtr(errorMessage)
	h.DownloadPercentage = nullInt64ToIntPtr(downloadPct)
	h.DownloadSpeed = nullStringToStrPtr(downloadSpeed)
	h.RowsProcessed = nullInt64ToIntPtr(rowsProcessed)
	h.DownloadCached = nullBoolToBoolPtr(downloadCached)
	h.DownloadDuration = nullInt64ToIntPtr(downloadDuration)
	h.ImportDuration = nullInt64ToIntPtr(importDuration)
	h.FileSize = nullInt64ToInt64Ptr(fileSize)
	h.TotalFiles = nullInt64ToIntPtr(totalFiles)
	h.CurrentFileIndex = nullInt64ToIntPtr(currentFileIndex)
	h.FilesProcessed = nullInt64ToIntPtr(filesProcessed)
	h.FileNames = nullStringToStrPtr(fileNames)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h)
}

func abortImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeProblem(w, http.StatusMethodNotAllowed, "Method Not Allowed", "POST or DELETE method required")
		return
	}

	ctx := context.Background()
	jobID := r.PathValue("job_id")

	if jobID == "" {
		writeProblem(w, http.StatusBadRequest, "Bad Request", "Job ID is required")
		return
	}

	result, err := db.ExecContext(ctx, `
		UPDATE import_history 
		SET status = 'failed', error_message = 'Aborted by user', completed_at = NOW() 
		WHERE job_id = $1 AND status IN ('importing', 'downloading')
	`, jobID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "Failed to abort import: "+err.Error())
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeProblem(w, http.StatusNotFound, "Not Found", "No active import job found with that ID")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func createImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "Method Not Allowed", "POST method required")
		return
	}

	ctx := context.Background()

	var active int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_history WHERE status IN ('importing', 'downloading')`).Scan(&active)
	if active > 0 {
		writeProblem(w, http.StatusConflict, "Conflict", "Import already in progress")
		return
	}

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
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "Failed to create import job: "+err.Error())
		return
	}

	w.Header().Set("Location", "/imports/"+jobID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Import started", "job_id": jobID})

	go func(limit int) {
		ctx := context.Background()

		if isImportAborted(jobID) {
			logger.Info("Import aborted before start", "job_id", jobID)
			return
		}

		files, err := downloadNotesWithProgress(ctx, 7, jobID)
		if err != nil {
			setImportFailed(jobID, err.Error())
			return
		}

		if len(files) > 0 {
			date := strings.Split(files[0].FileName, "-notes-")[0]
			cleanupOldFiles(date)
		}

		if isImportAborted(jobID) {
			setImportFailed(jobID, "Aborted by user")
			return
		}

		if limit > 0 {
			for _, f := range files {
				logger.Info("Truncating file", "path", f.TSVPath, "limit", limit)
				if err := truncateTSV(f.TSVPath, limit); err != nil {
					logger.Warn("Failed to truncate file", "path", f.TSVPath, "error", err)
				}
			}
		}

		totalFiles := len(files)
		var totalRows int // Will hold the final count
		var expectedTotalRows int
		var totalSize int64

		for _, f := range files {
			totalSize += f.FileSize
			if lines, err := countTSVRows(f.TSVPath); err == nil {
				expectedTotalRows += lines
			}
		}

		var fileNames []string
		for _, f := range files {
			fileNames = append(fileNames, f.FileName)
		}
		fileNamesStr := strings.Join(fileNames, ",")

		db.ExecContext(ctx, `UPDATE import_history SET status = 'importing', download_percentage = 100, total_rows = $1, file_size = $2, import_started_at = NOW(), files_processed = 0, file_names = $3 WHERE job_id = $4`, expectedTotalRows, totalSize, fileNamesStr, jobID)

		if isImportAborted(jobID) {
			setImportFailed(jobID, "Aborted by user")
			return
		}

		_, err = db.ExecContext(ctx, `TRUNCATE note`)
		if err != nil {
			setImportFailed(jobID, "failed to truncate table: "+err.Error())
			return
		}

		done := make(chan struct{})

		var cumulativeRows int
		var mu sync.Mutex

		go func() {
			for {
				select {
				case <-done:
					return
				case <-time.After(500 * time.Millisecond):
					var tuplesProcessed int
					err := db.QueryRowContext(context.Background(), `SELECT COALESCE(tuples_processed, 0) FROM pg_stat_progress_copy LIMIT 1`).Scan(&tuplesProcessed)
					if err == nil {
						mu.Lock()
						currentTotal := cumulativeRows + tuplesProcessed
						mu.Unlock()
						db.ExecContext(context.Background(), `UPDATE import_history SET rows_processed = $1, import_duration = EXTRACT(EPOCH FROM (NOW() - import_started_at))::INTEGER WHERE job_id = $2`, currentTotal, jobID)
					}
				}
			}
		}()

		for i, f := range files {
			if isImportAborted(jobID) {
				close(done)
				setImportFailed(jobID, "Aborted by user")
				return
			}

			db.ExecContext(ctx, `UPDATE import_history SET current_file_index = $1 WHERE job_id = $2`, i, jobID)

			res, err := db.ExecContext(ctx, fmt.Sprintf(`COPY note FROM '%s' WITH (FORMAT csv, DELIMITER E'\t', HEADER true)`, f.TSVPath))
			if err != nil {
				close(done)
				setImportFailed(jobID, "failed to import "+f.FileName+": "+err.Error())
				return
			}

			rowsAffected, _ := res.RowsAffected()
			logger.Info("COPY command output", "file", f.FileName, "rows_affected", rowsAffected)

			var count int
			err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM note`).Scan(&count)
			if err == nil {
				totalRows = count
				mu.Lock()
				cumulativeRows = count
				mu.Unlock()
			}

			db.ExecContext(ctx, `UPDATE import_history SET files_processed = $1 WHERE job_id = $2`, i+1, jobID)
			logger.Info("File imported", "file", f.FileName, "current", i+1, "total", totalFiles)
		}

		close(done)

		var importDuration int
		err = db.QueryRowContext(ctx, `SELECT EXTRACT(EPOCH FROM (NOW() - import_started_at))::INTEGER FROM import_history WHERE job_id = $1`, jobID).Scan(&importDuration)
		if err != nil {
			importDuration = 0
		}

		var dataDate string
		if len(files) > 0 {
			dataDate = strings.Split(files[0].FileName, "-notes-")[0]
			dataDate = fmt.Sprintf("20%s-%s-%s", dataDate[0:2], dataDate[2:4], dataDate[4:6])
		}

		_, err = db.ExecContext(ctx, `UPDATE import_history SET status = 'completed', total_rows = $1, completed_at = NOW(), import_duration = $2, data_date = $4 WHERE job_id = $3`, totalRows, importDuration, jobID, dataDate)
		if err != nil {
			logger.Warn("Failed to update status", "error", err)
		}

		logger.Info("Import completed", "rows", totalRows, "files", totalFiles)
	}(limit)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func getLatestAvailableDate(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	for i := 0; i < 7; i++ {
		date := getDateDaysAgo(i)
		url := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/notes-00000.zip",
			formatDateForURL(date))

		req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
		if err != nil {
			continue
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"date": date})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "no data found in last 7 days"})
}

func getLastImportDate(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var dataDate string
	err := db.QueryRowContext(ctx, `
		SELECT data_date::text FROM import_history
		WHERE status = 'completed' AND data_date IS NOT NULL
		ORDER BY completed_at DESC LIMIT 1
	`).Scan(&dataDate)

	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no completed imports found"})
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "Failed to query: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"date": dataDate})
}

func getSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	scheduler.mu.RLock()
	defer scheduler.mu.RUnlock()

	ctx := context.Background()
	var lastDataDate string
	db.QueryRowContext(ctx, `
		SELECT data_date::text FROM import_history
		WHERE status = 'completed' AND data_date IS NOT NULL
		ORDER BY completed_at DESC LIMIT 1
	`).Scan(&lastDataDate)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":        autoImportEnabled,
		"interval":       autoImportInterval.String(),
		"last_check":     scheduler.lastCheck,
		"next_run":       scheduler.nextRun,
		"last_data_date": lastDataDate,
	})
}
