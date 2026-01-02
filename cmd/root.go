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
var Version = "0.1.0" // Default version

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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sift.yaml)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// 1. Check local folder (Same as EXE) - Best for Dev
		exePath, err := os.Executable()
		if err == nil {
			viper.AddConfigPath(filepath.Dir(exePath))
		}

		// 2. Check Global ProgramData - Standard for Windows Services
		programData := os.Getenv("PROGRAMDATA")
		if programData != "" {
			viper.AddConfigPath(filepath.Join(programData, "Sift"))
		}

		// 3. Fallback to Home directory (Legacy)
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}

		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		// If we found one, lock it in so 'viper.WriteConfig()' updates the CORRECT file
		viper.SetConfigFile(viper.ConfigFileUsed())
	}
}
