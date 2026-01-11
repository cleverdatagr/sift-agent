package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusPending  = "PENDING"
	StatusUploaded = "UPLOADED"
	StatusVerified = "VERIFIED"
	StatusCorrupt  = "CORRUPT"
	StatusFailed   = "FAILED"
)

var dbInstance *sql.DB

func Init(dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	var err error
	dbInstance, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}

	// Create Table
	schema := `
	CREATE TABLE IF NOT EXISTS file_log (
		file_path TEXT PRIMARY KEY,
		file_hash TEXT,
		mod_time INTEGER,
		file_size INTEGER,
		status TEXT,
		last_attempt_at DATETIME,
		tenant_id TEXT,
		error_count INTEGER DEFAULT 0
	);
	`
	if _, err := dbInstance.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	return nil
}

func GetFileRecord(path string) (string, int64, string, int) {
	row := dbInstance.QueryRow("SELECT status, mod_time, file_hash, error_count FROM file_log WHERE file_path = ?", path)
	var status, hash string
	var modTime int64
	var errCount int
	if err := row.Scan(&status, &modTime, &hash, &errCount); err != nil {
		if err == sql.ErrNoRows {
			return "", 0, "", 0
		}
		log.Printf("DB Read Error: %v", err)
		return "", 0, "", 0
	}
	return status, modTime, hash, errCount
}

func UpdateFileStatus(path string, status string, hash string, modTime int64, size int64) {
	_, err := dbInstance.Exec(`
		INSERT INTO file_log (file_path, file_hash, mod_time, file_size, status, last_attempt_at, error_count)
		VALUES (?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(file_path) DO UPDATE SET
			status = excluded.status,
			file_hash = excluded.file_hash,
			mod_time = excluded.mod_time,
			file_size = excluded.file_size,
			last_attempt_at = excluded.last_attempt_at,
			error_count = 0
	`, path, hash, modTime, size, status, time.Now())

	if err != nil {
		log.Printf("DB Write Error: %v", err)
	}
}

func IncrementError(path string) {
	_, err := dbInstance.Exec("UPDATE file_log SET error_count = error_count + 1, last_attempt_at = ? WHERE file_path = ?", time.Now(), path)
	if err != nil {
		log.Printf("DB Error Increment Failed: %v", err)
	}
}

func MarkCorrupt(path string) {
	_, err := dbInstance.Exec("UPDATE file_log SET status = ?, last_attempt_at = ? WHERE file_path = ?", StatusCorrupt, time.Now(), path)
	if err != nil {
		log.Printf("DB Mark Corrupt Failed: %v", err)
	}
}

func ResetHistory(targetPath string) {
	var err error
	if targetPath != "" {
		_, err = dbInstance.Exec("DELETE FROM file_log WHERE file_path = ?", targetPath)
	} else {
		_, err = dbInstance.Exec("DELETE FROM file_log")
	}

	if err != nil {
		log.Printf("Failed to reset history: %v", err)
	} else {
		log.Println("History reset successfully.")
	}
}
