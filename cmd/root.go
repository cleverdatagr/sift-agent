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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var localMode bool
var debugMode bool
var Version = "0.2.0" // Default version

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sift",
	Short: "Sift Edge Agent",
	Long: `The Sift Edge Agent watches local folders and uploads documents 
to the Sift Intelligent Document Platform.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().BoolVar(&localMode, "local", false, "force use of local directory for config and database")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable excessive debug logging")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else if localMode {
		// --- LOCAL MODE: Use EXE folder ---
		exePath, err := os.Executable()
		if err != nil {
			fmt.Println("Error: Could not determine executable path")
			os.Exit(1)
		}
		viper.AddConfigPath(filepath.Dir(exePath))
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	} else {
		// --- GLOBAL MODE: Use ProgramData (Windows) or /etc (Linux) ---
		var globalDir string
		if os.Getenv("OS") == "Windows_NT" {
			globalDir = filepath.Join(os.Getenv("ProgramData"), "Sift")
		} else {
			globalDir = "/etc/sift"
		}

		// Ensure directory exists
		if _, err := os.Stat(globalDir); os.IsNotExist(err) {
			err := os.MkdirAll(globalDir, 0755)
			if err != nil {
				fmt.Printf("Error: Could not create global config directory at %s\n", globalDir)
				fmt.Println("Hint: Run as Administrator or use --local for development.")
				os.Exit(1)
			}
		}

		viper.AddConfigPath(globalDir)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		// If we are in Global Mode but no config exists, we should set the default file path
		// so that 'viper.WriteConfig()' creates it in the right place.
		viper.SetConfigFile(filepath.Join(globalDir, "config.yaml"))
	}

	viper.AutomaticEnv()

	// Only read if it exists
	if err := viper.ReadInConfig(); err == nil {
		// If we found one, lock it in so 'viper.WriteConfig()' updates the CORRECT file
		viper.SetConfigFile(viper.ConfigFileUsed())
	}
}
