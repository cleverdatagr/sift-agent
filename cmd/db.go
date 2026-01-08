// Copyright 2026 CleverData
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
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

func initDB() {
	var dbPath string

	// Check Config/Viper first
	if viper.IsSet("db_path") {
		dbPath = viper.GetString("db_path")
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			log.Fatalf("Failed to create db directory: %v", err)
		}
	} else {
		// Fallback to Standard OS Paths
		// Windows: %PROGRAMDATA%\CleverData\SiftAgent
		// Linux: /var/lib/sift-agent
		var dataDir string
		if os.Getenv("OS") == "Windows_NT" {
			dataDir = filepath.Join(os.Getenv("ProgramData"), "CleverData", "SiftAgent")
		} else {
			dataDir = "/var/lib/sift-agent"
		}

		if err := os.MkdirAll(dataDir, 0755); err != nil {
			log.Fatalf("Failed to create data directory: %v", err)
		}
		dbPath = filepath.Join(dataDir, "state.db")
	}
	
	var err error
	dbInstance, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database at %s: %v", dbPath, err)
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
		log.Fatalf("Failed to initialize schema: %v", err)
	}
}

func getFileRecord(path string) (string, int64, string, int) {
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

func updateFileStatus(path string, status string, hash string, modTime int64, size int64) {
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

func incrementError(path string) {
	_, err := dbInstance.Exec("UPDATE file_log SET error_count = error_count + 1, last_attempt_at = ? WHERE file_path = ?", time.Now(), path)
	if err != nil {
		log.Printf("DB Error Increment Failed: %v", err)
	}
}

func markCorrupt(path string) {
	_, err := dbInstance.Exec("UPDATE file_log SET status = ?, last_attempt_at = ? WHERE file_path = ?", StatusCorrupt, time.Now(), path)
	if err != nil {
		log.Printf("DB Mark Corrupt Failed: %v", err)
	}
}

func resetHistory(targetPath string) {
	// If path is provided, only clear that specific file
	// If empty, clear everything (Nuclear option)
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
