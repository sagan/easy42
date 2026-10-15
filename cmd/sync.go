package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/engine"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	syncPassword string
	dryRun       bool
	syncNode     string
	syncScope    string
)

func resolveScope(scope string, isTerminal bool) (string, error) {
	s := strings.ToLower(strings.TrimSpace(scope))
	switch s {
	case "auto":
		if isTerminal {
			return "full", nil
		}
		return "public", nil
	case "public", "full":
		return s, nil
	default:
		return "", fmt.Errorf("invalid scope: %s (must be 'auto', 'public', or 'full')", scope)
	}
}

func findRunningServerURL(dataDir string) string {
	var candidates []string
	if env := os.Getenv("EASY42_SERVER_URL"); env != "" {
		candidates = append(candidates, strings.TrimRight(env, "/"))
	}
	if urlBytes, err := os.ReadFile(filepath.Join(dataDir, "server.url")); err == nil {
		u := strings.TrimSpace(string(urlBytes))
		if u != "" {
			candidates = append(candidates, strings.TrimRight(u, "/"))
		}
	}
	candidates = append(candidates, "http://127.0.0.1:4242", "http://localhost:4242")

	client := &http.Client{Timeout: 500 * time.Millisecond}
	seen := make(map[string]bool)
	for _, u := range candidates {
		if seen[u] {
			continue
		}
		seen[u] = true
		resp, err := client.Get(u + "/api/auth/status")
		if err == nil && resp.StatusCode == http.StatusOK {
			var st struct {
				HasConfig bool `json:"has_config"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&st); err == nil && st.HasConfig {
				_ = resp.Body.Close()
				return u
			}
			_ = resp.Body.Close()
		}
	}
	return ""
}

func runSyncViaServer(serverURL string, cfg *config.Config, resolvedScope string, targetNodes []string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	sessionToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 15*time.Minute)

	if resolvedScope == "full" {
		statusResp, err := client.Get(serverURL + "/api/auth/status")
		if err == nil && statusResp.StatusCode == http.StatusOK {
			var st struct {
				Unlocked bool `json:"unlocked"`
			}
			_ = json.NewDecoder(statusResp.Body).Decode(&st)
			_ = statusResp.Body.Close()

			if !st.Unlocked {
				pass := syncPassword
				if pass == "" {
					pass = os.Getenv("EASY42_PASSWORD")
				}
				if pass == "" {
					fmt.Print("Enter easy42 admin password: ")
					bytePass, err := terminal.ReadPassword(int(syscall.Stdin))
					fmt.Println()
					if err != nil {
						reader := bufio.NewReader(os.Stdin)
						pass, _ = reader.ReadString('\n')
						pass = strings.TrimSpace(pass)
					} else {
						pass = strings.TrimSpace(string(bytePass))
					}
				}

				unlockPayload, _ := json.Marshal(map[string]string{"password": pass})
				req, err := http.NewRequest("POST", serverURL+"/api/auth/unlock", bytes.NewReader(unlockPayload))
				if err != nil {
					return err
				}
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(&http.Cookie{Name: "easy42_session", Value: sessionToken})
				uResp, err := client.Do(req)
				if err != nil {
					return fmt.Errorf("failed to authenticate with easy42 server: %w", err)
				}
				defer uResp.Body.Close()
				if uResp.StatusCode != http.StatusOK {
					var errMap map[string]string
					_ = json.NewDecoder(uResp.Body).Decode(&errMap)
					if msg, ok := errMap["error"]; ok && msg != "" {
						return fmt.Errorf("authentication failed: %s", msg)
					}
					return fmt.Errorf("authentication failed (HTTP %d)", uResp.StatusCode)
				}
			}
		}
	}

	params := url.Values{}
	params.Set("scope", resolvedScope)
	if len(targetNodes) > 0 {
		params.Set("node", strings.Join(targetNodes, ","))
	}

	if dryRun {
		req, err := http.NewRequest("GET", serverURL+"/api/sync/preview?"+params.Encode(), nil)
		if err != nil {
			return err
		}
		req.AddCookie(&http.Cookie{Name: "easy42_session", Value: sessionToken})
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to query sync preview: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errMap map[string]string
			_ = json.NewDecoder(resp.Body).Decode(&errMap)
			if msg, ok := errMap["error"]; ok && msg != "" {
				return fmt.Errorf("sync preview failed: %s", msg)
			}
			return fmt.Errorf("sync preview failed (HTTP %d)", resp.StatusCode)
		}

		var actions []config.SyncAction
		if err := json.NewDecoder(resp.Body).Decode(&actions); err != nil {
			return fmt.Errorf("failed to decode sync preview response: %w", err)
		}

		fmt.Printf("=== Sync Preview (%d actions planned) ===\n\n", len(actions))
		for i, act := range actions {
			fmt.Printf("[%d] %s -> %s\n", i+1, act.NodeName, act.Description)
			fmt.Printf("    Target file: %s\n", act.TargetFile)
		}
		return nil
	}

	if len(targetNodes) > 0 {
		fmt.Printf("Applying configuration to node %s...\n", targetNodes[0])
	} else {
		fmt.Println("Applying configuration across mesh nodes...")
	}

	req, err := http.NewRequest("POST", serverURL+"/api/sync?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.AddCookie(&http.Cookie{Name: "easy42_session", Value: sessionToken})
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute sync: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errMap map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errMap)
		if msg, ok := errMap["error"]; ok && msg != "" {
			return fmt.Errorf("sync execution failed: %s", msg)
		}
		return fmt.Errorf("sync execution failed (HTTP %d)", resp.StatusCode)
	}

	var results []config.SyncResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return fmt.Errorf("failed to decode sync results: %w", err)
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
}

var syncCmd = &cobra.Command{
	Use:     "sync",
	Aliases: []string{"apply"},
	Short:   "Synchronize WireGuard mesh topology configurations to remote nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		resolvedScope, err := resolveScope(syncScope, terminal.IsTerminal(int(os.Stdin.Fd())))
		if err != nil {
			return err
		}

		store := config.NewStore(GetDataDir())
		if !store.Exists() {
			return fmt.Errorf("easy42 is not initialized. Run 'easy42 serve' first")
		}

		cfg, err := store.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		var targetNodes []string
		if syncNode != "" {
			targetNodes = append(targetNodes, syncNode)
		}

		runningServerURL := findRunningServerURL(GetDataDir())
		if runningServerURL != "" {
			return runSyncViaServer(runningServerURL, cfg, resolvedScope, targetNodes)
		}

		// Standalone execution when server is not running
		mgr := engine.NewManager(store)
		defer mgr.Close()

		if resolvedScope == "full" {
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
		}

		if dryRun {
			actions, err := mgr.PlanSyncWithScope(resolvedScope, targetNodes...)
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

		if len(targetNodes) > 0 {
			fmt.Printf("Applying configuration to node %s...\n", targetNodes[0])
		} else {
			fmt.Println("Applying configuration across mesh nodes...")
		}
		results, err := mgr.ExecuteSyncNodesWithScope(false, resolvedScope, targetNodes...)
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
	syncCmd.Flags().StringVarP(&syncScope, "scope", "s", "auto", "Sync scope: auto (pty: full, non-pty: public), public, or full")
	syncCmd.Flags().StringVarP(&syncPassword, "password", "p", "", "easy42 password (or use EASY42_PASSWORD env)")
	syncCmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Preview actions without applying changes")
	syncCmd.Flags().StringVar(&syncNode, "node", "", "Target node name to sync (optional, default syncs all nodes)")
	RootCmd.AddCommand(syncCmd)
}
