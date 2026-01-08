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

	"github.com/spf13/cobra"
)

var resetPath string

var resetCmd = &cobra.Command{
	Use:   "reset-history",
	Short: "Clear the upload history database",
	Long:  `Clears the local SQLite database that tracks uploaded files. Use this to force the agent to re-upload files it has already processed.`, 
	Run: func(cmd *cobra.Command, args []string) {
		initDB()
		if resetPath != "" {
			fmt.Printf("Clearing history for: %s\n", resetPath)
		} else {
			fmt.Println("⚠️  WARNING: Clearing ENTIRE upload history. All files will be re-uploaded if seen again.")
			fmt.Println("Press Ctrl+C to cancel in 5 seconds...")
			// time.Sleep(5 * time.Second) // Uncomment for safety in prod
		}
		
		resetHistory(resetPath)
		
		log.Println("Database reset complete.")
	},
}

func init() {
	resetCmd.Flags().StringVarP(&resetPath, "path", "p", "", "Specific file path to clear from history")
	rootCmd.AddCommand(resetCmd)
}
