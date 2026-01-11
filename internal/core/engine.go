package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cleverdata/sift-agent/internal/api"
	"github.com/cleverdata/sift-agent/internal/config"
	"github.com/cleverdata/sift-agent/internal/db"
	"github.com/fsnotify/fsnotify"
)

var DebugMode bool

type fileState struct {
	lastSize int64
	lastMod  int64
	timer    *time.Timer
}

type Logger interface {
	Info(v ...interface{}) error
	Infof(format string, v ...interface{}) error
	Error(v ...interface{}) error
	Errorf(format string, v ...interface{}) error
	Warning(v ...interface{}) error
	Warningf(format string, v ...interface{}) error
}

func debugLog(logger Logger, format string, v ...interface{}) {
	if DebugMode && logger != nil {
		logger.Infof("[DEBUG] "+format, v...)
	}
}

func WatchRemote(ctx context.Context, remote config.RemoteConfig, logger Logger) {
	msg := fmt.Sprintf("[%s] Starting watcher on: %s", remote.Name, remote.Path)
	if logger != nil {
		logger.Info(msg)
	}

	// Ensure directory exists
	if _, err := os.Stat(remote.Path); os.IsNotExist(err) {
		msg := fmt.Sprintf("[%s] Creating directory: %s", remote.Name, remote.Path)
		if logger != nil {
			logger.Info(msg)
		}
		os.MkdirAll(remote.Path, 0755)
	}

	// --- PIPELINE CHANNELS ---
	type event struct {
		path string
		size int64
		mod  int64
	}
	eventChan := make(chan event, 100)
	doneChan := make(chan string, 100)

	// --- ORCHESTRATOR ---
	// Single goroutine that manages processing state and timers
	go func() {
		activeProcessing := make(map[string]bool)
		pendingStates := make(map[string]*fileState)

		limit := remote.ConcurrencyLimit
		if limit <= 0 {
			limit = 5
		}
		semaphore := make(chan struct{}, limit)

		settling, err := time.ParseDuration(remote.SettlingDelay)
		if err != nil {
			settling = 5 * time.Second
		}

		for {
			select {
			case e := <-eventChan:
				if activeProcessing[e.path] {
					debugLog(logger, "Ignoring event for %s: Already in worker pool", filepath.Base(e.path))
					continue
				}

				state, exists := pendingStates[e.path]
				if exists {
					// METADATA CHECK: Only reset timer if file actually changed
					if e.size != state.lastSize || e.mod != state.lastMod {
						debugLog(logger, "Metadata changed for %s (%d bytes -> %d bytes). Resetting timer.", filepath.Base(e.path), state.lastSize, e.size)
						state.timer.Stop()
						state.lastSize = e.size
						state.lastMod = e.mod

						// Start a fresh timer
						pathCopy := e.path // Capture for closure
						state.timer = time.AfterFunc(settling, func() {
							// Move from Pending to Active
							doneChan <- "START:" + pathCopy
						})
					} else {
						debugLog(logger, "Redundant event for %s: Metadata identical. Keeping current timer.", filepath.Base(e.path))
					}
				} else {
					debugLog(logger, "New file discovered: %s (%d bytes). Starting settling timer.", filepath.Base(e.path), e.size)
					newState := &fileState{
						lastSize: e.size,
						lastMod:  e.mod,
					}
					pathCopy := e.path
					newState.timer = time.AfterFunc(settling, func() {
						doneChan <- "START:" + pathCopy
					})
					pendingStates[e.path] = newState
				}

			case msg := <-doneChan:
				if strings.HasPrefix(msg, "START:") {
					path := strings.TrimPrefix(msg, "START:")
					delete(pendingStates, path)
					activeProcessing[path] = true

					debugLog(logger, "Settling period over for %s. Dispatching to worker pool.", filepath.Base(path))

					// Dispatch to worker pool
					go func(p string) {
						semaphore <- struct{}{} // Acquire slot
						debugLog(logger, "Worker slot ACQUIRED for %s", filepath.Base(p))

						defer func() {
							<-semaphore // Release slot
							debugLog(logger, "Worker slot RELEASED for %s", filepath.Base(p))
							doneChan <- "FINISH:" + p
						}()
						handleUpload(ctx, remote, p, logger)
					}(path)
				} else if strings.HasPrefix(msg, "FINISH:") {
					path := strings.TrimPrefix(msg, "FINISH:")
					delete(activeProcessing, path)
					debugLog(logger, "Processing cycle COMPLETE for %s", filepath.Base(path))
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	// Helper to probe a file and send an event
	probeAndSend := func(path string) {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return
		}
		if filepath.Base(path)[0] == '.' {
			return
		}

		abs, _ := filepath.Abs(path)
		eventChan <- event{
			path: abs,
			size: info.Size(),
			mod:  info.ModTime().UnixNano(),
		}
	}

	// --- INPUT SOURCE 1: FSNOTIFY (Real-time) ---
	if !remote.DisableFsnotify {
		go func() {
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				return
			}
			defer watcher.Close()
			watcher.Add(remote.Path)

			for {
				select {
				case e, ok := <-watcher.Events:
					if !ok {
						return
					}
					if e.Op&(fsnotify.Create|fsnotify.Write) != 0 {
						debugLog(logger, "FSNOTIFY event (%v) for %s", e.Op, filepath.Base(e.Name))
						probeAndSend(e.Name)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	} else {
		if logger != nil {
			logger.Infof("[%s] FSNOTIFY disabled. Running in polling-only mode.", remote.Name)
		}
	}

	// Poller
	go func() {
		pollInterval, err := time.ParseDuration(remote.PollingInterval)
		if err != nil {
			pollInterval = 1 * time.Minute
		}

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				debugLog(logger, "[%s] Starting backup directory scan...", remote.Name)
				files, _ := os.ReadDir(remote.Path)
				for _, f := range files {
					probeAndSend(filepath.Join(remote.Path, f.Name()))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Initial scan
	files, _ := os.ReadDir(remote.Path)
	for _, f := range files {
		probeAndSend(filepath.Join(remote.Path, f.Name()))
	}

	<-ctx.Done()
}

func handleUpload(ctx context.Context, remote config.RemoteConfig, absPath string, logger Logger) {
	info, err := os.Stat(absPath)
	if err != nil {
		return
	}

	status, dbModTime, _, errorCount := db.GetFileRecord(absPath)
	if errorCount > 10 {
		return
	}

	if (status == db.StatusUploaded || status == db.StatusVerified) && dbModTime == info.ModTime().UnixNano() {
		moveToDone(absPath, remote, logger)
		return
	}

	// --- STABILITY LOOP (Final Verification) ---
	threshold := remote.StabilityThreshold
	if threshold <= 0 {
		threshold = 2
	} // Lower default since we already passed SettlingDelay

	checkInt, _ := time.ParseDuration(remote.CheckInterval)
	if checkInt == 0 {
		checkInt = 5 * time.Second
	}

	maxWait, _ := time.ParseDuration(remote.StabilityTimeout)
	if maxWait == 0 {
		maxWait = 30 * time.Minute
	}

	lastSize := info.Size()
	stableCount := 0
	startTime := time.Now()

	for stableCount < threshold {
		if time.Since(startTime) > maxWait {
			if logger != nil {
				logger.Errorf("[%s] Stability Timeout: %s", remote.Name, filepath.Base(absPath))
			}
			return
		}

		select {
		case <-time.After(checkInt):
			inf, err := os.Stat(absPath)
			if err != nil {
				debugLog(logger, "Stability check error for %s: %v", filepath.Base(absPath), err)
				return
			}

			// Growth Check
			if inf.Size() != lastSize {
				debugLog(logger, "Stability FAILED for %s: Size changed (%d -> %d). Resetting loop.", filepath.Base(absPath), lastSize, inf.Size())
				lastSize = inf.Size()
				stableCount = 0
				continue
			}

			// Lock Probe
			f, err := os.OpenFile(absPath, os.O_RDWR, 0)
			if err != nil {
				debugLog(logger, "Stability FAILED for %s: File is LOCKED/BUSY. Resetting loop.", filepath.Base(absPath))
				stableCount = 0
				continue
			}
			f.Close()

			stableCount++
			debugLog(logger, "Stability Check PASSED (%d/%d) for %s", stableCount, threshold, filepath.Base(absPath))
		case <-ctx.Done():
			return
		}
	}

	if logger != nil {
		logger.Infof("[%s] Uploading: %s", remote.Name, filepath.Base(absPath))
	}

	onSuccess := func(path string, hash string, modTime int64) {
		db.UpdateFileStatus(path, db.StatusVerified, hash, modTime, 0)
		moveToDone(path, remote, logger)
	}

	onError := func(path string) {
		db.IncrementError(path)
	}

	api.UploadFile(ctx, remote, absPath, info.ModTime().UnixNano(), onSuccess, onError, func(f string, v ...interface{}) {
		if logger != nil {
			logger.Warningf(f, v...)
		}
	})
}

func moveToDone(absPath string, remote config.RemoteConfig, logger Logger) {
	doneDir := filepath.Join(filepath.Dir(absPath), ".done")
	os.MkdirAll(doneDir, 0755)

	dest := filepath.Join(doneDir, filepath.Base(absPath))
	if _, err := os.Stat(dest); err == nil {
		dest = filepath.Join(doneDir, fmt.Sprintf("%d_%s", time.Now().Unix(), filepath.Base(absPath)))
	}

	if err := os.Rename(absPath, dest); err == nil {
		if logger != nil {
			logger.Infof("[%s] Success: %s moved to .done", remote.Name, filepath.Base(absPath))
		}
	}
}