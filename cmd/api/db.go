package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

var (
	db         *sql.DB
	dbHost     = getEnv("DB_HOST", "localhost")
	dbPort     = "5432"
	dbUser     = "postgres"
	dbPassword = getEnv("DB_PASSWORD", "")
	dbName     = "postgres"
)

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

		db.SetMaxOpenConns(3)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(5 * time.Minute)

		if err = db.Ping(); err != nil {
			time.Sleep(delay)
			continue
		}

		return nil
	}
	return fmt.Errorf("failed to connect after %d retries: %w", maxRetries, err)
}
