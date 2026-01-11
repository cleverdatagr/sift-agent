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
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/cleverdata/sift-agent/internal/api"
	"github.com/cleverdata/sift-agent/internal/config"
	"github.com/cleverdata/sift-agent/internal/core"
	"github.com/cleverdata/sift-agent/internal/db"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func RunAgent() {
	initConfig()
	core.DebugMode = debugMode

	// 1. Setup Lifecycle Context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Initialize Database
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
		os.MkdirAll(dataDir, 0755)
		dbPath = filepath.Join(dataDir, "state.db")
	}

	if err := db.Init(dbPath); err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}

	// 3. Setup Logger
	svcConfig := &service.Config{Name: "SiftAgent"}
	prg := &program{}
	s, err := service.New(prg, svcConfig)
	var logger service.Logger
	if err == nil {
		logger, _ = s.Logger(nil)
	}

	msg := "Sift Agent Starting..."
	fmt.Println(msg)
	if logger != nil {
		logger.Info(msg)
	}

	// 4. Load Remotes
	var remotes []config.RemoteConfig
	if err := viper.UnmarshalKey("remotes", &remotes); err != nil {
		if logger != nil {
			logger.Errorf("Error parsing config: %v", err)
		}
		return
	}

	if len(remotes) == 0 {
		idle := "No remotes configured. Idling..."
		fmt.Println(idle)
		if logger != nil {
			logger.Info(idle)
		}
		// Wait for signal even when idling to avoid deadlock
		<-ctx.Done()
		return
	}

	// 5. Start Pipeline
	var wg sync.WaitGroup
	for _, r := range remotes {
		wg.Add(1)
		go func(remote config.RemoteConfig) {
			defer wg.Done()
			
			// Heartbeat
			go api.Pinger(ctx, remote, func(f string, v ...interface{}) {
				if logger != nil {
					logger.Warningf(f, v...)
				}
			})

			// Watcher Engine
			core.WatchRemote(ctx, remote, logger)
		}(r)
	}

	// Wait for signal or all workers to stop
	<-ctx.Done()
	fmt.Println("Sift Agent shutting down...")
	wg.Wait()
}
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent in the foreground (Internal Use)",
	Long:  `Runs the watcher process directly. Usually invoked by the Windows Service.`,
	Run: func(cmd *cobra.Command, args []string) {
		if service.Interactive() {
			RunAgent()
		} else {
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
