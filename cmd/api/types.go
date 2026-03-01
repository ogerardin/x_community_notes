package main

import (
	"context"
	"io"
	"time"
)

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
	FileSize           *int64     `json:"file_size,omitempty"`
	TotalFiles         *int       `json:"total_files,omitempty"`
	CurrentFileIndex   *int       `json:"current_file_index,omitempty"`
	FilesProcessed     *int       `json:"files_processed,omitempty"`
	FileNames          *string    `json:"file_names,omitempty"`
	IndexingStartedAt  *time.Time `json:"indexing_started_at,omitempty"`
	IndexPhase         *string    `json:"index_phase,omitempty"`
	IndexBlocksDone    *int       `json:"index_blocks_done,omitempty"`
	IndexBlocksTotal   *int       `json:"index_blocks_total,omitempty"`
}

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
	FileSize           *int64     `json:"file_size,omitempty"`
	ImportDuration     *int       `json:"import_duration,omitempty"`
	TotalFiles         *int       `json:"total_files,omitempty"`
	CurrentFileIndex   *int       `json:"current_file_index,omitempty"`
	FilesProcessed     *int       `json:"files_processed,omitempty"`
}

type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type FileInfo struct {
	ZipPath  string
	TSVPath  string
	FileName string
	FileSize int64
}

type progressTracker struct {
	reader           io.Reader
	totalBytes       int64
	bytesRead        int64
	lastUpdate       time.Time
	lastPct          int
	startTime        time.Time
	ctx              context.Context
	jobID            string
	fileName         string
	totalFiles       int
	currentFileIndex int
}
