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

				ctx := context.Background()

				latestReq, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+port+"/imports/latest-available", nil)
				if err != nil {
					logger.Warn("Failed to create latest-available request", "error", err)
					continue
				}

				latestResp, err := http.DefaultClient.Do(latestReq)
				if err != nil {
					logger.Warn("Failed to check latest-available", "error", err)
					continue
				}

				var latest struct {
					Date string `json:"date"`
				}
				if err := json.NewDecoder(latestResp.Body).Decode(&latest); err != nil || latest.Date == "" {
					latestResp.Body.Close()
					logger.Warn("Failed to decode latest-available response")
					continue
				}
				latestResp.Body.Close()

				lastReq, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+port+"/imports/last-import-date", nil)
				if err != nil {
					logger.Warn("Failed to create last-import-date request", "error", err)
					continue
				}

				lastResp, err := http.DefaultClient.Do(lastReq)
				if err != nil {
					logger.Warn("Failed to check last-import-date", "error", err)
					continue
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

					createReq, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:"+port+"/imports", nil)
					if err != nil {
						logger.Warn("Failed to create import request", "error", err)
						continue
					}

					createResp, err := http.DefaultClient.Do(createReq)
					if err != nil {
						logger.Warn("Failed to trigger import", "error", err)
						continue
					}
					createResp.Body.Close()
				} else {
					logger.Info("No new data available", "latest", latest.Date, "last", last.Date)
				}
			}
		}
	}()
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
	http.HandleFunc("GET /imports/{job_id}", getImportByID)
	http.HandleFunc("POST /imports", createImport)
	http.HandleFunc("POST /imports/{job_id}/abort", abortImport)
	http.HandleFunc("DELETE /imports/{job_id}", abortImport)
	http.HandleFunc("GET /imports/latest-available", getLatestAvailableDate)
	http.HandleFunc("GET /imports/last-import-date", getLastImportDate)
	http.HandleFunc("GET /imports/scheduler", getSchedulerStatus)

	startAutoImporter()

	logger.Info("Starting API server", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
