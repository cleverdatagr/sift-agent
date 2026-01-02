package cmd

import (
	"fmt"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Service definition
type program struct{}

func (p *program) Start(s service.Service) error {
	// Start the actual work in a non-blocking way
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Cleanup logic here
	return nil
}

func (p *program) run() {
	// This is where the actual "Run" command logic will eventually live
	// For now, we just redirect to the run command logic
	RunAgent()
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

		svcConfig := &service.Config{
			Name:        "SiftAgent",
			DisplayName: "Sift Intelligent Document Agent",
			Description: "Watches configured folders and uploads documents to Sift IDP.",
			// Arguments to pass to the service when it starts
			Arguments: []string{"run", "--config", configPath},
		}

		prg := &program{}
		s, err := service.New(prg, svcConfig)
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

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(statusCmd)
}
