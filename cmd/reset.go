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
	"log"
	"os"
	"path/filepath"

	"github.com/cleverdata/sift-agent/internal/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var resetPath string

var resetCmd = &cobra.Command{
	Use:   "reset-history",
	Short: "Clear the upload history database",
	Long:  `Clears the local SQLite database that tracks uploaded files. Use this to force the agent to re-upload files it has already processed.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Initialize DB first
		var dbPath string
		if viper.IsSet("db_path") {
			dbPath = viper.GetString("db_path")
		} else if localMode {
			exePath, _ := os.Executable()
			dbPath = filepath.Join(filepath.Dir(exePath), "state.db")
		} else {
			var dataDir string
			if os.Getenv("OS") == "Windows_NT" {
				dataDir = filepath.Join(os.Getenv("ProgramData"), "Sift")
			} else {
				dataDir = "/var/lib/sift-agent"
			}
			dbPath = filepath.Join(dataDir, "state.db")
		}
		db.Init(dbPath)

		if resetPath != "" {
			fmt.Printf("Clearing history for: %s\n", resetPath)
		} else {
			fmt.Println("⚠️  WARNING: Clearing ENTIRE upload history. All files will be re-uploaded if seen again.")
			fmt.Println("Press Ctrl+C to cancel in 5 seconds...")
		}

		db.ResetHistory(resetPath)

		log.Println("Database reset complete.")
	},
}

func init() {
	resetCmd.Flags().StringVarP(&resetPath, "path", "p", "", "Specific file path to clear from history")
	rootCmd.AddCommand(resetCmd)
}
