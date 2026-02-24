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

func getCurrentImport(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	if currentJobID == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ImportStatus{})
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
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "Failed to get import status: "+err.Error())
		return
	}

	status.TotalRows = &totalRows
	status.StartedAt = nullTimeToTimePtr(startedAt)
	status.CompletedAt = nullTimeToTimePtr(completedAt)
	status.ErrorMessage = nullStringToStrPtr(errorMessage)
	status.DownloadPercentage = nullInt64ToIntPtr(downloadPct)
	status.DownloadSpeed = nullStringToStrPtr(downloadSpeed)
	status.FileName = nullStringToStrPtr(fileName)
	status.FileSize = nullInt64ToInt64Ptr(fileSize)
	status.ImportDuration = nullInt64ToIntPtr(importDuration)
	status.TotalFiles = nullInt64ToIntPtr(totalFiles)
	status.CurrentFileIndex = nullInt64ToIntPtr(currentFileIndex)
	status.FilesProcessed = nullInt64ToIntPtr(filesProcessed)

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

func listImports(w http.ResponseWriter, r *http.Request) {
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
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "Failed to get import history: "+err.Error())
		return
	}
	defer rows.Close()

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

		h.CompletedAt = nullTimeToTimePtr(completedAt)
		h.TotalRows = nullInt64ToIntPtr(totalRows)
		h.ErrorMessage = nullStringToStrPtr(errorMessage)
		h.DownloadPercentage = nullInt64ToIntPtr(downloadPct)
		h.DownloadSpeed = nullStringToStrPtr(downloadSpeed)
		h.RowsProcessed = nullInt64ToIntPtr(rowsProcessed)
		h.DownloadCached = nullBoolToBoolPtr(downloadCached)
		h.DownloadDuration = nullInt64ToIntPtr(downloadDuration)
		h.ImportDuration = nullInt64ToIntPtr(importDuration)
		h.FileName = nullStringToStrPtr(fileName)
		h.FileSize = nullInt64ToInt64Ptr(fileSize)
		h.TotalFiles = nullInt64ToIntPtr(totalFiles)
		h.CurrentFileIndex = nullInt64ToIntPtr(currentFileIndex)
		h.FilesProcessed = nullInt64ToIntPtr(filesProcessed)
		h.FileNames = nullStringToStrPtr(fileNames)

		history = append(history, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
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
	var fileName sql.NullString
	var fileSize sql.NullInt64
	var totalFiles sql.NullInt64
	var currentFileIndex sql.NullInt64
	var filesProcessed sql.NullInt64
	var fileNames sql.NullString

	err := db.QueryRowContext(ctx, `
		SELECT id, job_id, started_at, completed_at, total_rows, status, error_message, 
		       download_percentage, download_speed, rows_processed, download_cached, download_duration, import_duration, file_name, file_size,
		       total_files, current_file_index, files_processed, file_names
		FROM import_history 
		WHERE job_id = $1
	`, jobID).Scan(&h.ID, &h.JobID, &h.StartedAt, &completedAt, &totalRows, &h.Status, &errorMessage, &downloadPct, &downloadSpeed, &rowsProcessed, &downloadCached, &downloadDuration, &importDuration, &fileName, &fileSize, &totalFiles, &currentFileIndex, &filesProcessed, &fileNames)

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
	h.FileName = nullStringToStrPtr(fileName)
	h.FileSize = nullInt64ToInt64Ptr(fileSize)
	h.TotalFiles = nullInt64ToIntPtr(totalFiles)
	h.CurrentFileIndex = nullInt64ToIntPtr(currentFileIndex)
	h.FilesProcessed = nullInt64ToIntPtr(filesProcessed)
	h.FileNames = nullStringToStrPtr(fileNames)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h)
}

func createImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "Method Not Allowed", "POST method required")
		return
	}

	if currentJobID != nil {
		writeProblem(w, http.StatusConflict, "Conflict", "Import already in progress")
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
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "Failed to create import job: "+err.Error())
		return
	}

	currentJobID = &jobID

	w.Header().Set("Location", "/imports/"+jobID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Import started", "job_id": jobID})

	go func(limit int) {
		ctx := context.Background()

		files, err := downloadNotesWithProgress(ctx, 7, jobID)
		if err != nil {
			setImportFailed(jobID, err.Error())
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

		db.ExecContext(ctx, `UPDATE import_history SET status = 'importing', download_percentage = 100, total_rows = $1, file_name = $2, file_size = $3, import_started_at = NOW(), files_processed = 0, file_names = $4 WHERE job_id = $5`, expectedTotalRows, fmt.Sprintf("%d files", totalFiles), totalSize, fileNamesStr, jobID)

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
			db.ExecContext(ctx, `UPDATE import_history SET current_file_index = $1, file_name = $2 WHERE job_id = $3`, i, f.FileName, jobID)

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

		_, err = db.ExecContext(ctx, `UPDATE import_history SET status = 'completed', total_rows = $1, completed_at = NOW(), import_duration = $2 WHERE job_id = $3`, totalRows, importDuration, jobID)
		if err != nil {
			logger.Warn("Failed to update status", "error", err)
		}

		currentJobID = nil
		logger.Info("Import completed", "rows", totalRows, "files", totalFiles)
	}(limit)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
