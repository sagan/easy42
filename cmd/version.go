package cmd

import (
	"easy42/internal/version"
	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print easy42 version and build information",
	Long:  `Print comprehensive version, commit, build date, and bundled agent details.`,
	Run: func(cmd *cobra.Command, args []string) {
		info := version.GetInfo()
		cmd.Printf("easy42 %s\n", info.Version)
		if info.Commit != "" && info.Commit != "none" {
			cmd.Printf("  Git commit:     %s\n", info.Commit)
		}
		if info.Date != "" && info.Date != "unknown" {
			cmd.Printf("  Built at:       %s\n", info.Date)
		}
		if info.BuiltBy != "" && info.BuiltBy != "unknown" {
			cmd.Printf("  Built by:       %s\n", info.BuiltBy)
		}
		cmd.Printf("  Go version:     %s\n", info.GoVersion)
		cmd.Printf("  OS/Arch:        %s\n", info.Platform)
		cmd.Printf("  Bundled agent:  v%s\n", info.AgentVersion)
		cmd.Printf("  Project URL:    %s\n", info.GitHubURL)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
	RootCmd.Version = version.Version
}
