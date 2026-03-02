package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	logger             *slog.Logger
	port               = "8888"
	autoImportEnabled  = getEnvBool("AUTO_IMPORT_ENABLED", true)
	autoImportInterval = getEnvDuration("AUTO_IMPORT_INTERVAL", time.Hour)
)

type schedulerState struct {
	mu        sync.RWMutex
	lastCheck time.Time
	nextRun   time.Time
}

var scheduler = &schedulerState{}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func startAutoImporter() {
	if !autoImportEnabled {
		logger.Info("Auto-import scheduler disabled")
		return
	}

	checkAndImport := func() {
		ctx := context.Background()

		latestReq, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+port+"/admin/imports/latest-available", nil)
		if err != nil {
			logger.Warn("Failed to create latest-available request", "error", err)
			return
		}

		latestResp, err := http.DefaultClient.Do(latestReq)
		if err != nil {
			logger.Warn("Failed to check latest-available", "error", err)
			return
		}

		var latest struct {
			Date string `json:"date"`
		}
		if err := json.NewDecoder(latestResp.Body).Decode(&latest); err != nil || latest.Date == "" {
			latestResp.Body.Close()
			logger.Warn("Failed to decode latest-available response")
			return
		}
		latestResp.Body.Close()

		lastReq, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+port+"/admin/imports/last-import-date", nil)
		if err != nil {
			logger.Warn("Failed to create last-import-date request", "error", err)
			return
		}

		lastResp, err := http.DefaultClient.Do(lastReq)
		if err != nil {
			logger.Warn("Failed to check last-import-date", "error", err)
			return
		}

		var last struct {
			Date string `json:"date"`
		}
		if err := json.NewDecoder(lastResp.Body).Decode(&last); err != nil {
			lastResp.Body.Close()
			logger.Info("No previous import found, triggering import", "latest", latest.Date)
			last.Date = ""
		}
		lastResp.Body.Close()

		if latest.Date > last.Date {
			logger.Info("New data available, triggering import", "latest", latest.Date, "last", last.Date)

			createReq, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:"+port+"/admin/imports", nil)
			if err != nil {
				logger.Warn("Failed to create import request", "error", err)
				return
			}

			createResp, err := http.DefaultClient.Do(createReq)
			if err != nil {
				logger.Warn("Failed to trigger import", "error", err)
				return
			}
			createResp.Body.Close()
		} else {
			logger.Info("No new data available", "latest", latest.Date, "last", last.Date)
			_, err := db.ExecContext(ctx, `INSERT INTO import_history (started_at, status, data_date) VALUES (NOW(), 'skipped', $1)`, latest.Date)
			if err != nil {
				logger.Warn("Failed to insert skipped record", "error", err)
			}
		}
	}

	var lastImportTime time.Time
	err := db.QueryRowContext(context.Background(), `SELECT COALESCE(MAX(COALESCE(data_date::timestamp, started_at)), '1970-01-01') FROM import_history WHERE status = 'completed'`).Scan(&lastImportTime)
	if err != nil {
		logger.Warn("Failed to get last import time", "error", err)
	} else if time.Since(lastImportTime) >= autoImportInterval {
		logger.Info("Last import older than interval, checking for updates", "lastImport", lastImportTime, "interval", autoImportInterval)
		checkAndImport()
	}

	ticker := time.NewTicker(autoImportInterval)
	scheduler.lastCheck = time.Now()
	scheduler.nextRun = scheduler.lastCheck.Add(autoImportInterval)
	logger.Info("Auto-update scheduler started", "interval", autoImportInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				scheduler.mu.Lock()
				scheduler.lastCheck = time.Now()
				scheduler.nextRun = scheduler.lastCheck.Add(autoImportInterval)
				scheduler.mu.Unlock()

				checkAndImport()
			}
		}
	}()
}

func getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetVersionInfo())
}

func main() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := initDBWithRetry(30, time.Second); err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	sanitizeImportStatus()

	http.HandleFunc("/health", healthCheck)
	http.HandleFunc("/version", getVersion)
	http.HandleFunc("GET /admin/imports/current", getImportCurrent)
	http.HandleFunc("GET /admin/imports/{job_id}", getImportByID)
	http.HandleFunc("POST /admin/imports", createImport)
	http.HandleFunc("POST /admin/imports/{job_id}/abort", abortImport)
	http.HandleFunc("DELETE /admin/imports/{job_id}", abortImport)
	http.HandleFunc("GET /admin/imports/latest-available", getLatestAvailableDate)
	http.HandleFunc("GET /admin/imports/last-import-date", getLastImportDate)
	http.HandleFunc("GET /admin/imports/scheduler", getSchedulerStatus)

	logger.Info("Starting API server", "port", port)
	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			logger.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	time.Sleep(time.Second)
	startAutoImporter()

	select {
	case <-make(chan struct{}):
	}
}
