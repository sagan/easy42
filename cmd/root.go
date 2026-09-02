package cmd

import (
	"fmt"
	"os"

	"easy42/internal/config"
	"github.com/spf13/cobra"
)

var (
	dataDir string
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "easy42",
	Short: "easy42 - WireGuard Overlay Mesh Network Manager for DN42 and Private Networks",
	Long: `easy42 is an agent-less, WireGuard overlay networking tool designed for
managing internal node meshes with Linux kernel WireGuard, link-local IPv6,
and automatic configuration synchronization.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&dataDir, "data-dir", "d", config.DefaultDataDir(), "Path to easy42 data directory")
}

func GetDataDir() string {
	return dataDir
}
