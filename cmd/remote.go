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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cleverdata/sift-agent/internal/config"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote endpoints and watch folders",
}

var remoteAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new folder to watch",
	Long: `Adds a local folder to the agent's watch list using a robust pipeline architecture. 
	
The agent monitors folders using both real-time events (Watcher) and periodic scans (Poller).
Files must pass a "Settling Delay" (initial silence) and a "Stability Loop" (final verification) 
before being uploaded.

Total Verification Time ≈ settling-delay + (stability-threshold * check-interval).
Stability Timeout = Maximum time to wait for a file to stop changing (default 30m).
Concurrency Limit = Max simultaneous uploads per folder (default 5).
Polling Interval  = Frequency of the backup directory scan (default 1m).`,
	Example: `  sift remote add --name scans --path "C:\Scans" --endpoint "https://api.sift.com" --key "sk_..." --concurrency-limit 10 --settling-delay 10s`,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		path, _ := cmd.Flags().GetString("path")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		key, _ := cmd.Flags().GetString("key")
		force, _ := cmd.Flags().GetBool("force")
		stabilityThreshold, _ := cmd.Flags().GetInt("stability-threshold")
		checkInterval, _ := cmd.Flags().GetString("check-interval")
		stabilityTimeout, _ := cmd.Flags().GetString("stability-timeout")
		concurrencyLimit, _ := cmd.Flags().GetInt("concurrency-limit")
		pollingInterval, _ := cmd.Flags().GetString("polling-interval")
		settlingDelay, _ := cmd.Flags().GetString("settling-delay")
		noFsnotify, _ := cmd.Flags().GetBool("no-fsnotify")

		if name == "" || path == "" || key == "" {
			fmt.Println("Error: --name, --path, and --key are required.")
			return
		}

		// Normalize endpoint (remove trailing slash)
		endpoint = strings.TrimRight(endpoint, "/")

		// --- VERIFICATION STEP ---
		if !force {
			fmt.Printf("Verifying connection to %s...\n", endpoint)
			client := resty.New()
			resp, err := client.R().
				SetHeader("Authorization", "Bearer "+key).
				Get(endpoint + "/agent/check")

			if err != nil {
				fmt.Printf("❌ Connection Failed: %v\n", err)
				fmt.Println("Use --force to add anyway.")
				return
			}

			if resp.StatusCode() == 401 || resp.StatusCode() == 403 {
				fmt.Printf("❌ Authentication Failed: Invalid API Key (Status: %d)\n", resp.StatusCode())
				return
			}

			if resp.StatusCode() != 200 {
				fmt.Printf("❌ Unexpected Response: Status %d - %s\n", resp.StatusCode(), resp.String())
				return
			}

			fmt.Println("✅ Connection Verified!")
		}
		// -------------------------

		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Printf("Invalid path: %v\n", err)
			return
		}

		// Load existing remotes
		var remotes []config.RemoteConfig
		if err := viper.UnmarshalKey("remotes", &remotes); err != nil {
			remotes = []config.RemoteConfig{}
		}

		// Check for duplicates
		for _, r := range remotes {
			if r.Name == name {
				fmt.Printf("Error: Remote '%s' already exists.\n", name)
				return
			}
		}

		newRemote := config.RemoteConfig{
			Name:               name,
			Path:               absPath,
			Endpoint:           endpoint,
			Key:                key,
			StabilityThreshold: stabilityThreshold,
			CheckInterval:      checkInterval,
			StabilityTimeout:   stabilityTimeout,
			ConcurrencyLimit:   concurrencyLimit,
			PollingInterval:    pollingInterval,
			SettlingDelay:      settlingDelay,
			DisableFsnotify:    noFsnotify,
		}

		remotes = append(remotes, newRemote)
		viper.Set("remotes", remotes)

		// Save config
		if viper.ConfigFileUsed() != "" {
			if err := viper.WriteConfig(); err != nil {
				fmt.Printf("Failed to update config: %v\n", err)
				return
			}
		} else {
			// No config exists yet, let's create one in the best location
			targetDir := ""
			isAdmin := checkIfAdmin()

			if isAdmin {
				targetDir = filepath.Join(os.Getenv("PROGRAMDATA"), "Sift")
			} else {
				exePath, _ := os.Executable()
				targetDir = filepath.Dir(exePath)
				fmt.Println("\n>>> NOTE: Running as non-admin. Config saved to local folder.")
				fmt.Println(">>> The Windows Service will NOT see this remote.")
			}

			os.MkdirAll(targetDir, 0755)
			viper.SetConfigFile(filepath.Join(targetDir, "config.yaml"))

			if err := viper.SafeWriteConfig(); err != nil {
				fmt.Printf("Failed to create config: %v\n", err)
				return
			}
		}

				fmt.Printf("Remote '%s' added successfully. Watching: %s\n", name, absPath)
				fmt.Printf("Policy: %d checks @ %s | Max Wait: %s | Workers: %d | Polling: %s | Settling: %s\n", 
					stabilityThreshold, checkInterval, stabilityTimeout, concurrencyLimit, pollingInterval, settlingDelay)
				if noFsnotify {
					fmt.Println("Mode: POLLING ONLY (Real-time events disabled)")
				} else {
					fmt.Println("Mode: REAL-TIME (fsnotify) + Polling Backup")
				}
				fmt.Println("\n>>> IMPORTANT: Run 'sift restart' to apply these changes to the running service.") 
			},
		}
func checkIfAdmin() bool {
	// Simple Windows-only check for Admin rights
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	return err == nil
}

var remoteListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List configured remotes",
	Run: func(cmd *cobra.Command, args []string) {
		var remotes []config.RemoteConfig
		viper.UnmarshalKey("remotes", &remotes)

		if len(remotes) == 0 {
			fmt.Println("No remotes configured.")
			return
		}

		fmt.Printf("% -15s % -40s %s\n", "NAME", "PATH", "ENDPOINT")
		fmt.Println("--------------------------------------------------------------------------------")
		for _, r := range remotes {
			fmt.Printf("% -15s % -40s %s\n", r.Name, r.Path, r.Endpoint)
		}
	},
}

var remoteRemoveCmd = &cobra.Command{
	Use:     "remove [name]",
	Aliases: []string{"rm", "del"},
	Short:   "Remove a configured remote",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		var remotes []config.RemoteConfig
		if err := viper.UnmarshalKey("remotes", &remotes); err != nil {
			fmt.Println("No remotes configured.")
			return
		}

		found := false
		var updatedRemotes []config.RemoteConfig
		for _, r := range remotes {
			if r.Name == name {
				found = true
				continue
			}
			updatedRemotes = append(updatedRemotes, r)
		}

		if !found {
			fmt.Printf("Error: Remote '%s' not found.\n", name)
			return
		}

		viper.Set("remotes", updatedRemotes)
		if err := viper.WriteConfig(); err != nil {
			fmt.Printf("Failed to save config: %v\n", err)
			return
		}

		fmt.Printf("Remote '%s' removed successfully.\n", name)
		fmt.Println("\n>>> IMPORTANT: Run 'sift restart' to apply these changes to the running service.")
	},
}

func init() {
	remoteAddCmd.Flags().String("name", "", "Unique name for this watcher")
	remoteAddCmd.Flags().String("path", "", "Local folder path to watch")
	remoteAddCmd.Flags().String("endpoint", "https://sift.cleverdata.gr/api/v1", "API Endpoint URL")
	remoteAddCmd.Flags().String("key", "", "API Key (Secret)")
	remoteAddCmd.Flags().Bool("force", false, "Skip connection verification")
	remoteAddCmd.Flags().Int("stability-threshold", 3, "Number of consecutive checks that must pass (default: 3)")
	remoteAddCmd.Flags().String("check-interval", "5s", "Time to wait between checks (default: 5s)")
	remoteAddCmd.Flags().String("stability-timeout", "30m", "Maximum time to wait for stability (default: 30m)")
	remoteAddCmd.Flags().Int("concurrency-limit", 5, "Maximum number of simultaneous uploads (default: 5)")
	remoteAddCmd.Flags().String("polling-interval", "1m", "Interval for the backup scan (default: 1m)")
	remoteAddCmd.Flags().String("settling-delay", "5s", "Wait for silence before verification starts (default: 5s)")
	remoteAddCmd.Flags().Bool("no-fsnotify", false, "Disable real-time filesystem events (rely purely on polling)")

	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
	rootCmd.AddCommand(remoteCmd)
}
