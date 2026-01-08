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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-resty/resty/v2"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func RunAgent() {
	initConfig() // Ensure config is loaded (Critical for Service mode)
	initDB()     // Initialize SQLite

	// Setup System Logger (Event Viewer on Windows)
	svcConfig := &service.Config{Name: "SiftAgent"}
	prg := &program{}
	s, err := service.New(prg, svcConfig)
	var logger service.Logger
	if err == nil {
		logger, _ = s.Logger(nil)
	}

	// ALWAYS LOG STARTUP
	msg := "Sift Agent Starting..."
	fmt.Println(msg)
	if logger != nil {
		logger.Info(msg)
	}

	// reload config just in case
	if err := viper.ReadInConfig(); err != nil {
		warn := fmt.Sprintf("Config not found or invalid: %v", err)
		fmt.Println(warn)
		if logger != nil {
			logger.Warning(warn)
		}
	}

	var remotes []RemoteConfig
	if err := viper.UnmarshalKey("remotes", &remotes); err != nil {
		err_msg := fmt.Sprintf("Error parsing config: %v", err)
		fmt.Println(err_msg)
		if logger != nil {
			logger.Error(err_msg)
		}
		return
	}

	if len(remotes) == 0 {
		idle := "No remotes configured. Idling..."
		fmt.Println(idle)
		if logger != nil {
			logger.Info(idle)
		}
		select {} // Block forever
	}

	var wg sync.WaitGroup

	for _, remote := range remotes {
		wg.Add(1)
		go func(r RemoteConfig) {
			defer wg.Done()
			
			// Start background heartbeat
			go pinger(r, logger)
			
			watchRemote(r, logger)
		}(remote)
	}

	wg.Wait()
}

func pinger(remote RemoteConfig, logger service.Logger) {
	client := resty.New()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		resp, err := client.R().
			SetHeader("Authorization", "Bearer "+remote.Key).
			Get(remote.Endpoint + "/agent/check")

		if err != nil {
			if logger != nil {
				logger.Warningf("[%s] Heartbeat failed: %v", remote.Name, err)
			}
		} else if resp.StatusCode() != 200 {
			if logger != nil {
				logger.Warningf("[%s] Heartbeat rejected: Status %d", remote.Name, resp.StatusCode())
			}
		}
	}
}

func watchRemote(remote RemoteConfig, logger service.Logger) {
	msg := fmt.Sprintf("[%s] Starting watcher on: %s", remote.Name, remote.Path)
	fmt.Println(msg)
	if logger != nil {
		logger.Info(msg)
	}

	// Ensure directory exists
	if _, err := os.Stat(remote.Path); os.IsNotExist(err) {
		msg := fmt.Sprintf("[%s] Creating directory: %s", remote.Name, remote.Path)
		fmt.Println(msg)
		if logger != nil {
			logger.Info(msg)
		}
		os.MkdirAll(remote.Path, 0755)
	}

	// --- NEW: Scan for existing files before watching ---
	files, err := os.ReadDir(remote.Path)
	if err == nil {
		for _, file := range files {
			if !file.IsDir() && filepath.Base(file.Name())[0] != '.' {
				fullPath := filepath.Join(remote.Path, file.Name())
				// Ensure absolute path
				absPath, _ := filepath.Abs(fullPath)
				msg := fmt.Sprintf("[%s] Found existing file: %s", remote.Name, file.Name())
				fmt.Println(msg)
				if logger != nil {
					logger.Info(msg)
				}
				go handleUpload(remote, absPath, logger)
			}
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		err_msg := fmt.Sprintf("[%s] Error creating watcher: %v", remote.Name, err)
		fmt.Println(err_msg)
		if logger != nil {
			logger.Error(err_msg)
		}
		return
	}
	defer watcher.Close()

	done := make(chan bool)

	// Processing Loop
	go func() {
		pendingUploads := make(map[string]*time.Timer)
		var mu sync.Mutex

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					if filepath.Base(event.Name)[0] == '.' {
						continue
					}

					mu.Lock()
					if t, exists := pendingUploads[event.Name]; exists {
						t.Stop()
					}

					pendingUploads[event.Name] = time.AfterFunc(1*time.Second, func() {
						mu.Lock()
						delete(pendingUploads, event.Name)
						mu.Unlock()
						
						// Ensure absolute path
						absPath, _ := filepath.Abs(event.Name)
						handleUpload(remote, absPath, logger)
					})
					mu.Unlock()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if logger != nil {
					logger.Errorf("[%s] Watcher error: %v", remote.Name, err)
				}
			}
		}
	}()

	if err := watcher.Add(remote.Path); err != nil {
		if logger != nil {
			logger.Errorf("[%s] Failed to add path: %v", remote.Name, err)
		}
		return
	}

	<-done
}

func handleUpload(remote RemoteConfig, filePath string, logger service.Logger) {
	// filePath is already absolute from the caller
	info, err := os.Stat(filePath)
	if err != nil {
		return
	}

	if info.IsDir() {
		return
	}

	status, dbModTime, _, errorCount := getFileRecord(filePath)
	
	if errorCount > 10 {
		if logger != nil {
			logger.Warningf("[%s] Skipping %s: Too many errors", remote.Name, filepath.Base(filePath))
		}
		return
	}

	if (status == StatusUploaded || status == StatusVerified) && dbModTime == info.ModTime().UnixNano() {
		// --- SELF-HEALING MOVE ---
		doneDir := filepath.Join(filepath.Dir(filePath), ".done")
		if _, err := os.Stat(doneDir); os.IsNotExist(err) {
			os.Mkdir(doneDir, 0755)
		}
		
		destPath := filepath.Join(doneDir, filepath.Base(filePath))
		if _, err := os.Stat(destPath); err == nil {
			timestamp := time.Now().Format("20060102150405")
			destPath = filepath.Join(doneDir, fmt.Sprintf("%s_%s", timestamp, filepath.Base(filePath)))
		}

		if err := os.Rename(filePath, destPath); err == nil {
			if logger != nil {
				logger.Infof("[%s] Cleanup move successful: %s", remote.Name, filepath.Base(filePath))
			}
		}
		return
	}

	initialSize := info.Size()
	time.Sleep(500 * time.Millisecond)
	info2, err := os.Stat(filePath)
	if err != nil || info2.Size() != initialSize {
		return
	}

	if logger != nil {
		logger.Infof("[%s] Uploading: %s", remote.Name, filepath.Base(filePath))
	}

	uploadFile(remote, filePath, info.ModTime().UnixNano(), logger)
}

func uploadFile(remote RemoteConfig, filePath string, modTime int64, logger service.Logger) {
	client := resty.New()
	
	absPath, _ := filepath.Abs(filePath)
	doneDir := filepath.Join(filepath.Dir(absPath), ".done")
	if _, err := os.Stat(doneDir); os.IsNotExist(err) {
		err := os.Mkdir(doneDir, 0755)
		if err != nil && logger != nil {
			logger.Errorf("[%s] FAILED to create .done directory: %v", remote.Name, err)
		}
	}

	f, err := os.Open(absPath)
	if err != nil {
		return
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return
	}
	localHash := hex.EncodeToString(hasher.Sum(nil))

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		resp, err := client.R().
			SetHeader("Authorization", "Bearer " + remote.Key).
			SetFile("file", absPath).
			Post(fmt.Sprintf("%s/agent/upload", remote.Endpoint))

		if err == nil && resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
			updateFileStatus(absPath, StatusVerified, localHash, modTime, 0)
			
			destPath := filepath.Join(doneDir, filepath.Base(absPath))
			if _, err := os.Stat(destPath); err == nil {
				timestamp := time.Now().Format("20060102150405")
				destPath = filepath.Join(doneDir, fmt.Sprintf("%s_%s", timestamp, filepath.Base(absPath)))
			}

			// CLOSE FILE BEFORE RENAME (Windows Requirement)
			f.Close() 

			if err := os.Rename(absPath, destPath); err != nil {
				if logger != nil {
					logger.Errorf("[%s] FAILED to move file to .done: %v", remote.Name, err)
				}
			} else {
				if logger != nil {
					logger.Infof("[%s] Success: %s moved to .done", remote.Name, filepath.Base(absPath))
				}
			}
			return
		}
		time.Sleep(2 * time.Second)
	}
	incrementError(absPath)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent in the foreground (Internal Use)",
	Long:  `Runs the watcher process directly. Usually invoked by the Windows Service.`,
	Run: func(cmd *cobra.Command, args []string) {
		if service.Interactive() {
			RunAgent()
		} else {
			// When running as a service, we MUST call s.Run() to check-in with Windows SCM
			s, err := getService(viper.ConfigFileUsed())
			if err != nil {
				log.Fatalf("Failed to initialize service: %v", err)
			}
			s.Run()
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
