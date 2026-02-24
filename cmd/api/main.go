package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var (
	logger *slog.Logger
	port   = "8888"
)

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

	logger.Info("Starting API server", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
