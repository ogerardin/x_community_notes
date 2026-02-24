package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var (
	db         *sql.DB
	dbHost     = getEnv("DB_HOST", "localhost")
	dbPort     = "5432"
	dbUser     = "postgres"
	dbPassword = "example"
	dbName     = "postgres"
)

func initDBWithRetry(maxRetries int, delay time.Duration) error {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	for i := 0; i < maxRetries; i++ {
		connector, err := pq.NewConnector(dsn)
		if err != nil {
			time.Sleep(delay)
			continue
		}

		connectorWithNotice := pq.ConnectorWithNoticeHandler(connector, func(err *pq.Error) {
			if err != nil {
				logger.Info("Postgres Notice", "severity", err.Severity, "message", err.Message, "code", err.Code)
			}
		})

		db = sql.OpenDB(connectorWithNotice)

		if err = db.Ping(); err != nil {
			time.Sleep(delay)
			continue
		}

		if err := runMigrations(db); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		// Notify PostgREST to reload its schema cache after migrations
		db.ExecContext(context.Background(), "NOTIFY pgrst, 'reload schema'")

		return nil
	}
	return fmt.Errorf("failed to connect after %d retries: %w", maxRetries, err)
}
