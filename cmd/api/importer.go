package main

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var dataDir = "/home/data"

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
			`UPDATE import_history SET download_percentage = $1, download_speed = $2, download_duration = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER, file_size = $3, total_files = $4, current_file_index = $5 WHERE job_id = $6`,
			currentPct, speedStr, pt.totalBytes, pt.totalFiles, pt.currentFileIndex, pt.jobID)
	}

	return n, err
}

func discoverFileCount(ctx context.Context, date string) int {
	baseURL := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/", formatDateForURL(date))

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

func downloadNotesWithProgress(ctx context.Context, lookbackDays int, jobID string) ([]FileInfo, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	var date string
	var found bool
	for i := 0; i < lookbackDays; i++ {
		date = getDateDaysAgo(i)
		url := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/notes-00000.zip",
			formatDateForURL(date))

		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("no data files found in the last %d days", lookbackDays)
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
		filename := fmt.Sprintf("%s-%s", date, formatFileName(i)+".zip")
		filepath := filepath.Join(dataDir, filename)
		urlFilename := formatFileName(i) + ".zip"
		url := fmt.Sprintf("https://ton.twimg.com/birdwatch-public-data/%s/notes/%s",
			formatDateForURL(date), urlFilename)

		var fileSize int64
		var cached bool

		if _, err := os.Stat(filepath); err == nil {
			logger.Info("File already exists", "path", filepath)
			info, _ := os.Stat(filepath)
			fileSize = info.Size()
			cached = true

			db.ExecContext(ctx, `UPDATE import_history SET current_file_index = $1, file_size = $2, download_cached = $3, download_percentage = 100 WHERE job_id = $4`, i, fileSize, cached, jobID)
		} else {
			logger.Info("Downloading file", "url", url, "path", filepath)

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
			logger.Info("Downloaded file", "path", filepath)
		}

		db.ExecContext(ctx, `UPDATE import_history SET current_file_index = $1, file_size = $2, download_cached = $3 WHERE job_id = $4`, i, fileSize, cached, jobID)

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

			logger.Info("Extracted TSV", "path", tsvPath)
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

func setImportFailed(jobID, errMsg string) {
	db.ExecContext(context.Background(), `UPDATE import_history SET status = 'failed', error_message = $1, completed_at = NOW() WHERE job_id = $2`, errMsg, jobID)
}

func sanitizeImportStatus() {
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		UPDATE import_history 
		SET status = 'failed', error_message = 'Interrupted'
		WHERE status IN ('importing', 'downloading')
	`)
	if err != nil {
		logger.Warn("Failed to sanitize import status", "error", err)
		return
	}

	logger.Info("Cleared any running import jobs")
}

func cleanupOldFiles(keepDate string) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		logger.Warn("Failed to read data directory", "error", err)
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, keepDate) {
			path := filepath.Join(dataDir, name)
			if err := os.Remove(path); err != nil {
				logger.Warn("Failed to remove old file", "path", path, "error", err)
			} else {
				logger.Info("Removed old file", "path", path)
			}
		}
	}
}
