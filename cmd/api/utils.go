package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func nullInt64ToIntPtr(n sql.NullInt64) *int {
	if n.Valid {
		i := int(n.Int64)
		return &i
	}
	return nil
}

func nullInt64ToInt64Ptr(n sql.NullInt64) *int64 {
	if n.Valid {
		return &n.Int64
	}
	return nil
}

func nullStringToStrPtr(n sql.NullString) *string {
	if n.Valid {
		return &n.String
	}
	return nil
}

func nullTimeToTimePtr(n sql.NullTime) *time.Time {
	if n.Valid {
		return &n.Time
	}
	return nil
}

func nullBoolToBoolPtr(n sql.NullBool) *bool {
	if n.Valid {
		return &n.Bool
	}
	return nil
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Problem{
		Type:   fmt.Sprintf("https://httpstatuses.com/%d", status),
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

func getDateDaysAgo(n int) string {
	now := time.Now()
	date := now.AddDate(0, 0, -n)
	return date.Format("2006-01-02")
}

func formatDateForURL(date string) string {
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
