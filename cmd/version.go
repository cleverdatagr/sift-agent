package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Sift Agent",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Sift Agent v%s\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

