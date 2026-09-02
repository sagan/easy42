package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	syncPassword string
	dryRun       bool
)

var syncCmd = &cobra.Command{
	Use:     "sync",
	Aliases: []string{"apply"},
	Short:   "Synchronize WireGuard mesh topology configurations to remote nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.NewStore(GetDataDir())
		if !store.Exists() {
			return fmt.Errorf("easy42 is not initialized. Run 'easy42 serve' first")
		}

		if _, err := store.Load(); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		mgr := engine.NewManager(store)

		// Prompt for password if not provided
		pass := syncPassword
		if pass == "" {
			pass = os.Getenv("EASY42_PASSWORD")
		}
		if pass == "" {
			fmt.Print("Enter easy42 admin password: ")
			bytePass, err := terminal.ReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil {
				// Fallback to bufio if terminal is not a tty
				reader := bufio.NewReader(os.Stdin)
				pass, _ = reader.ReadString('\n')
				pass = strings.TrimSpace(pass)
			} else {
				pass = strings.TrimSpace(string(bytePass))
			}
		}

		if err := mgr.Unlock(pass); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		if dryRun {
			actions, err := mgr.PlanSync()
			if err != nil {
				return err
			}
			fmt.Printf("=== Sync Preview (%d actions planned) ===\n\n", len(actions))
			for i, act := range actions {
				fmt.Printf("[%d] %s -> %s\n", i+1, act.NodeName, act.Description)
				fmt.Printf("    Target file: %s\n", act.TargetFile)
			}
			return nil
		}

		fmt.Println("Applying configuration across mesh nodes...")
		results, err := mgr.ExecuteSync()
		if err != nil {
			return fmt.Errorf("sync execution failed: %w", err)
		}

		fmt.Printf("\n=== Sync Results (%d actions) ===\n", len(results))
		for _, r := range results {
			statusStr := "SUCCESS"
			if !r.Success {
				statusStr = fmt.Sprintf("FAILED (%s)", r.Error)
			}
			fmt.Printf("[%s] Node: %-12s Action: %s (%.0fms)\n", statusStr, r.NodeName, r.Action, r.Duration)
		}

		return nil
	},
}

func init() {
	syncCmd.Flags().StringVarP(&syncPassword, "password", "p", "", "easy42 password (or use EASY42_PASSWORD env)")
	syncCmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Preview actions without applying changes")
	RootCmd.AddCommand(syncCmd)
}
