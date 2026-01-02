package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RunAgent is the entry point for the long-running process
func RunAgent() {
	fmt.Println("Sift Agent Starting...")

	// reload config just in case
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: Config not found or invalid: %v", err)
	}

	var remotes []RemoteConfig
	if err := viper.UnmarshalKey("remotes", &remotes); err != nil {
		log.Printf("Error parsing config: %v", err)
		return
	}

	if len(remotes) == 0 {
		log.Println("No remotes configured. Idling...")
		select {} // Block forever
	}

	var wg sync.WaitGroup

	for _, remote := range remotes {
		wg.Add(1)
		go func(r RemoteConfig) {
			defer wg.Done()
			
			// Start background heartbeat
			go pinger(r)
			
			watchRemote(r)
		}(remote)
	}

	wg.Wait()
}

func pinger(remote RemoteConfig) {
	client := resty.New()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		resp, err := client.R().
			SetHeader("Authorization", "Bearer "+remote.Key).
			Get(remote.Endpoint + "/agent/check")

		if err != nil {
			log.Printf("[%s] Heartbeat failed: %v", remote.Name, err)
		} else if resp.StatusCode() != 200 {
			log.Printf("[%s] Heartbeat rejected: Status %d", remote.Name, resp.StatusCode())
		}
	}
}

func watchRemote(remote RemoteConfig) {
	log.Printf("[%s] Starting watcher on: %s", remote.Name, remote.Path)

	// Ensure directory exists
	if _, err := os.Stat(remote.Path); os.IsNotExist(err) {
		log.Printf("[%s] Directory does not exist, creating: %s", remote.Name, remote.Path)
		os.MkdirAll(remote.Path, 0755)
	}

	// --- NEW: Scan for existing files before watching ---
	files, err := os.ReadDir(remote.Path)
	if err == nil {
		for _, file := range files {
			if !file.IsDir() && filepath.Base(file.Name())[0] != '.' {
				fullPath := filepath.Join(remote.Path, file.Name())
				log.Printf("[%s] Found existing file: %s", remote.Name, file.Name())
				go handleUpload(remote, fullPath)
			}
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[%s] Error creating watcher: %v", remote.Name, err)
		return
	}
	defer watcher.Close()

	done := make(chan bool)

	// Processing Loop
	go func() {
		// Dedup/Debounce map: filename -> timer
		pendingUploads := make(map[string]*time.Timer)
		var mu sync.Mutex

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Only care about Create and Write
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					
					// Ignore hidden files or temp files if needed
					if filepath.Base(event.Name)[0] == '.' {
						continue
					}

					mu.Lock()
					// If timer exists, stop it (reset debounce)
					if t, exists := pendingUploads[event.Name]; exists {
						t.Stop()
					}

					// Start new timer (Stabilization Window: 1 second)
					// If no new write events happen for 1s, we assume file is ready.
					pendingUploads[event.Name] = time.AfterFunc(1*time.Second, func() {
						mu.Lock()
						delete(pendingUploads, event.Name)
						mu.Unlock()
						
						handleUpload(remote, event.Name)
					})
					mu.Unlock()
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[%s] Watcher error: %v", remote.Name, err)
			}
		}
	}()

	if err := watcher.Add(remote.Path); err != nil {
		log.Printf("[%s] Failed to add path: %v", remote.Name, err)
		return
	}

	<-done
}

func handleUpload(remote RemoteConfig, filePath string) {
	// Verify file exists and is accessible
	info, err := os.Stat(filePath)
	if err != nil {
		log.Printf("[%s] File vanished before upload: %s", remote.Name, filePath)
		return
	}

	if info.IsDir() {
		return
	}

	// Stability Check (Double-Check)
	// Some scanners create the file, then pause, then write again.
	// We check if size remains constant for another 500ms.
	initialSize := info.Size()
	time.Sleep(500 * time.Millisecond)
	info2, err := os.Stat(filePath)
	if err != nil || info2.Size() != initialSize {
		log.Printf("[%s] File %s is still changing (Size: %d -> %d). Retrying later...", remote.Name, filepath.Base(filePath), initialSize, info2.Size())
		// Reschedule logic could go here, for now we just skip to avoid partial uploads
		return
	}

	log.Printf("[%s] 🚀 Ready to upload: %s (%d bytes)", remote.Name, filepath.Base(filePath), info.Size())

	// Perform HTTP Upload
	uploadFile(remote, filePath)
}

func uploadFile(remote RemoteConfig, filePath string) {
	client := resty.New()
	
	// Prepare destination for "done" files
	doneDir := filepath.Join(filepath.Dir(filePath), ".done")
	if _, err := os.Stat(doneDir); os.IsNotExist(err) {
		os.Mkdir(doneDir, 0755)
	}

	// Retry loop
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		resp, err := client.R().
			SetHeader("Authorization", "Bearer " + remote.Key).
			SetFile("file", filePath).
			Post(fmt.Sprintf("%s/agent/upload", remote.Endpoint))

		if err == nil && resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
			log.Printf("[%s] ✅ Upload Success: %s (Response: %s)", remote.Name, filepath.Base(filePath), resp.String())
			
			// Move to .done
			destPath := filepath.Join(doneDir, filepath.Base(filePath))
			
			// Handle collisions in .done
			if _, err := os.Stat(destPath); err == nil {
				timestamp := time.Now().Format("20060102150405")
				destPath = filepath.Join(doneDir, fmt.Sprintf("%s_%s", timestamp, filepath.Base(filePath)))
			}

			if err := os.Rename(filePath, destPath); err != nil {
				log.Printf("[%s] Warning: Failed to move file to .done: %v", remote.Name, err)
			}
			return
		}

		log.Printf("[%s] ⚠️ Upload Attempt %d failed: %v (Status: %d). Retrying...", remote.Name, i+1, err, resp.StatusCode())
		time.Sleep(2 * time.Second)
	}

	log.Printf("[%s] ❌ Upload Failed after %d attempts: %s", remote.Name, maxRetries, filepath.Base(filePath))
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent in the foreground (Internal Use)",
	Long:  `Runs the watcher process directly. Usually invoked by the Windows Service.`,
	Run: func(cmd *cobra.Command, args []string) {
		RunAgent()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}