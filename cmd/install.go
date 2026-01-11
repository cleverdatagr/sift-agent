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
	"os/exec"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// program implements the service.Interface
type program struct{}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	return nil
}

func (p *program) run() {
	RunAgent()
}

func getService(configPath string) (service.Service, error) {
	args := []string{"run"}
	if localMode {
		args = append(args, "--local")
	} else if configPath != "" {
		args = append(args, "--config", configPath)
	}

	svcConfig := &service.Config{
		Name:        "SiftAgent",
		DisplayName: "Sift Intelligent Document Agent",
		Description: "Watches configured folders and uploads documents to Sift IDP.",
		Arguments:   args,
	}

	prg := &program{}
	return service.New(prg, svcConfig)
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the Sift Agent as a Windows Service",
	Run: func(cmd *cobra.Command, args []string) {
		// Find current config file to pass to the service
		configPath := viper.ConfigFileUsed()
		if configPath == "" {
			fmt.Println("Error: No config file found. Please run 'sift remote add' first.")
			return
		}

		s, err := getService(configPath)
		if err != nil {
			fmt.Printf("Setup failed: %v\n", err)
			return
		}

		// Check if already installed
		status, err := s.Status()
		if err == nil {
			fmt.Println("Sift Agent is already installed.")
			if status == service.StatusRunning {
				fmt.Println("Service is currently RUNNING.")
			} else {
				fmt.Println("Service is currently STOPPED.")
			}
			fmt.Println("Use 'sift restart' to apply config changes, or 'sift uninstall' to remove it.")
			return
		}

		fmt.Println("Installing Sift Agent Service...")
		if err := s.Install(); err != nil {
			fmt.Printf("Failed to install: %v\n", err)
			fmt.Println("Hint: Ensure you are running as Administrator.")
			return
		}
		fmt.Println("Service installed successfully.")

		fmt.Println("Starting service...")
		if err := s.Start(); err != nil {
			fmt.Printf("Failed to start: %v\n", err)
			return
		}
		fmt.Println("Service started.")
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the Sift Agent Service",
	Run: func(cmd *cobra.Command, args []string) {
		svcConfig := &service.Config{
			Name: "SiftAgent",
		}
		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		if err := s.Stop(); err != nil {
			// Ignore stop errors, it might not be running
		}

		if err := s.Uninstall(); err != nil {
			fmt.Printf("Failed to uninstall: %v\n", err)
			return
		}
		fmt.Println("Service uninstalled.")
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Sift Agent Service",
	Run: func(cmd *cobra.Command, args []string) {
		svcConfig := &service.Config{
			Name: "SiftAgent",
		}
		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("Restarting Sift Agent Service...")
		if err := s.Restart(); err != nil {
			fmt.Printf("Failed to restart: %v\n", err)
			return
		}
		fmt.Println("Service restarted.")
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Sift Agent Service",
	Run: func(cmd *cobra.Command, args []string) {
		svcConfig := &service.Config{
			Name: "SiftAgent",
		}
		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("Stopping Sift Agent Service...")
		if err := s.Stop(); err != nil {
			fmt.Printf("Failed to stop: %v\n", err)
			return
		}
		fmt.Println("Service stopped.")
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Sift Agent Service",
	Run: func(cmd *cobra.Command, args []string) {
		svcConfig := &service.Config{
			Name: "SiftAgent",
		}
		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("Starting Sift Agent Service...")
		if err := s.Start(); err != nil {
			fmt.Printf("Failed to start: %v\n", err)
			return
		}
		fmt.Println("Service started.")
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Sift Agent Service",
	Run: func(cmd *cobra.Command, args []string) {
		svcConfig := &service.Config{
			Name: "SiftAgent",
		}
		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		status, err := s.Status()
		if err != nil {
			fmt.Printf("Could not get status: %v\n", err)
			return
		}

		statusStr := "Unknown"
		switch status {
		case service.StatusRunning:
			statusStr = "Running"
		case service.StatusStopped:
			statusStr = "Stopped"
		}

		fmt.Printf("Sift Agent Service Status: %s\n", statusStr)
	},
}

var enableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable the Sift Agent to start automatically with Windows",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Enabling Sift Agent Service (Automatic Start)...")
		// We use standard Windows 'sc' command to set start type
		cmdExec := exec.Command("sc", "config", "SiftAgent", "start=", "auto")
		if err := cmdExec.Run(); err != nil {
			fmt.Printf("Failed to enable: %v\n", err)
			return
		}
		fmt.Println("Service enabled for automatic start.")
	},
}

var disableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable the Sift Agent from starting with Windows",
	Run: func(cmd *cobra.Command, args []string) {
		svcConfig := &service.Config{
			Name: "SiftAgent",
		}
		prg := &program{}
		s, err := service.New(prg, svcConfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("Stopping Sift Agent Service...")
		s.Stop()

		fmt.Println("Disabling Sift Agent Service (Manual Start Only)...")
		cmdExec := exec.Command("sc", "config", "SiftAgent", "start=", "demand")
		if err := cmdExec.Run(); err != nil {
			fmt.Printf("Failed to disable: %v\n", err)
			return
		}
		fmt.Println("Service disabled.")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(enableCmd)
	rootCmd.AddCommand(disableCmd)
}
