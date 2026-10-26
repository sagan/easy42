package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/engine"

	"github.com/spf13/cobra"
)

var (
	applyFilePath string
	applyName     string
	applyDryRun   bool
	applySync     bool
	applyFormat   string
)

var applyCmd = &cobra.Command{
	Use:   "apply [file | -]",
	Short: "Apply a partial config change snippet (JSON or YAML) to easy42",
	Long: `Applies a declarative config change snippet to easy42 configuration.
Supports partial node patches, link overrides, and network settings.
Performs strict JSON Schema verification before previewing or applying changes.
Snippets can be loaded from a file, standard input, or a saved snippet name with --name.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			applyDryRun = false
			applyFilePath = ""
			applyName = ""
			applySync = false
			applyFormat = "text"
		}()

		if cmd.Flags().Changed("name") && strings.TrimSpace(applyName) == "" {
			return fmt.Errorf("config change name cannot be empty")
		}

		if applyName != "" && (len(args) > 0 || applyFilePath != "") {
			return fmt.Errorf("cannot specify both a file/stdin and --name")
		}

		if applyName == "" && len(args) == 0 && applyFilePath == "" {
			return fmt.Errorf("specify a file path, '-' to read from stdin, or --name <name> (e.g. easy42 apply -f patch.yaml or easy42 apply --name my-snippet)")
		}

		store := config.NewStore(GetDataDir())
		cfg, err := store.Load()
		if err != nil {
			return fmt.Errorf("failed to load easy42 config: %w", err)
		}

		var content string
		if applyName != "" {
			var foundSnippet *config.ConfigChange
			for _, cc := range cfg.ConfigChanges {
				if cc.Name == applyName {
					foundSnippet = &cc
					break
				}
			}
			if foundSnippet == nil {
				for _, cc := range cfg.ConfigChanges {
					if strings.EqualFold(cc.Name, applyName) {
						foundSnippet = &cc
						break
					}
				}
			}
			if foundSnippet == nil {
				return fmt.Errorf("saved config change '%s' not found in config.json", applyName)
			}
			content = foundSnippet.Content
		} else {
			var targetSource string
			if len(args) > 0 {
				targetSource = args[0]
			} else {
				targetSource = applyFilePath
			}

			var rawBytes []byte
			if targetSource == "-" {
				rawBytes, err = io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read from stdin: %w", err)
				}
			} else {
				rawBytes, err = os.ReadFile(targetSource)
				if err != nil {
					return fmt.Errorf("failed to read file '%s': %w", targetSource, err)
				}
			}
			content = string(rawBytes)
		}

		if strings.TrimSpace(content) == "" {
			return fmt.Errorf("config change content is empty")
		}

		serverURL := findRunningServerURL(GetDataDir())

		if applyDryRun {
			return handleApplyPreview(content, serverURL, store, cfg)
		}

		return handleApplyExecute(content, serverURL, store, cfg)
	},
}

func handleApplyPreview(content, serverURL string, store *config.Store, cfg *config.Config) error {
	var preview *engine.ApplyPreviewResult
	var err error

	if serverURL != "" {
		preview, err = requestServerApplyPreview(serverURL, cfg, content)
	}
	if serverURL == "" || err != nil {
		mgr := engine.NewManager(store)
		preview, err = mgr.PreviewApply(content)
	}

	if err != nil {
		fmt.Printf("❌ Validation Failed:\n%v\n", err)
		return err
	}

	if applyFormat == "json" {
		out, _ := json.MarshalIndent(preview, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("\n🔍 Config Change Preview (Dry-Run):\n")
	fmt.Printf("Status: Valid (Detected format: %s)\n", preview.Format)
	fmt.Printf("Summary: %d modified node(s), %d added node(s), %d modified link(s), %d added link(s) (Total changes: %d)\n\n",
		preview.Summary.ModifiedNodes,
		preview.Summary.AddedNodes,
		preview.Summary.ModifiedLinks,
		preview.Summary.AddedLinks,
		preview.Summary.TotalChanges,
	)

	if len(preview.Nodes) > 0 {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NODE\tACTION\tFIELD\tOLD VALUE\tNEW VALUE")
		for _, nd := range preview.Nodes {
			for _, ch := range nd.Changes {
				fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%v\n", nd.Name, nd.Action, ch.Field, ch.OldValue, ch.NewValue)
			}
		}
		w.Flush()
		fmt.Println()
	}

	if len(preview.Links) > 0 {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "LINK\tACTION\tFIELD\tOLD VALUE\tNEW VALUE")
		for _, ld := range preview.Links {
			for _, ch := range ld.Changes {
				fmt.Fprintf(w, "%s <-> %s\t%s\t%s\t%v\t%v\n", ld.From, ld.To, ld.Action, ch.Field, ch.OldValue, ch.NewValue)
			}
		}
		w.Flush()
		fmt.Println()
	}

	if len(preview.Settings) > 0 {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "CATEGORY\tFIELD\tOLD VALUE\tNEW VALUE")
		for _, sd := range preview.Settings {
			for _, ch := range sd.Changes {
				fmt.Fprintf(w, "%s\t%s\t%v\t%v\n", sd.Category, ch.Field, ch.OldValue, ch.NewValue)
			}
		}
		w.Flush()
		fmt.Println()
	}

	if preview.UnifiedDiff != "" {
		fmt.Println("--- Unified Diff ---")
		fmt.Println(preview.UnifiedDiff)
	}

	return nil
}

func handleApplyExecute(content, serverURL string, store *config.Store, cfg *config.Config) error {
	var result *engine.ApplyResult
	var err error

	if serverURL != "" {
		result, err = requestServerApplyExecute(serverURL, cfg, content)
	}
	if serverURL == "" || err != nil {
		mgr := engine.NewManager(store)
		result, err = mgr.ApplyConfigChange(content)
	}

	if err != nil {
		fmt.Printf("❌ Apply Failed:\n%v\n", err)
		return err
	}

	fmt.Printf("✅ Successfully applied config change at %s\n", result.AppliedAt.Format(time.RFC3339))
	if len(result.AffectedNodes) > 0 {
		fmt.Printf("Affected Nodes: %s\n", strings.Join(result.AffectedNodes, ", "))
	}
	if len(result.AffectedLinks) > 0 {
		fmt.Printf("Affected Links: %s\n", strings.Join(result.AffectedLinks, ", "))
	}
	if result.BackupFile != "" {
		fmt.Printf("Config backup saved to: %s\n", result.BackupFile)
	}

	if applySync {
		fmt.Println("\n🔄 Triggering network sync...")
		if serverURL != "" {
			return runSyncViaServer(serverURL, cfg, "auto", result.AffectedNodes)
		}
		// Standalone sync
		mgr := engine.NewManager(store)
		_, syncErr := mgr.ExecuteSyncNodesWithScope(false, "auto", result.AffectedNodes...)
		if syncErr != nil {
			return fmt.Errorf("apply succeeded but sync failed: %w", syncErr)
		}
		fmt.Println("✅ Network sync completed.")
	}

	return nil
}

func requestServerApplyPreview(serverURL string, cfg *config.Config, content string) (*engine.ApplyPreviewResult, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	sessionToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 15*time.Minute)

	payload, _ := json.Marshal(map[string]string{
		"content": content,
	})
	req, err := http.NewRequest("POST", serverURL+"/api/config/apply/preview", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "easy42_session", Value: sessionToken})

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	var preview engine.ApplyPreviewResult
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		return nil, err
	}
	return &preview, nil
}

func requestServerApplyExecute(serverURL string, cfg *config.Config, content string) (*engine.ApplyResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	sessionToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 15*time.Minute)

	payload, _ := json.Marshal(map[string]interface{}{
		"content": content,
	})
	req, err := http.NewRequest("POST", serverURL+"/api/config/apply", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "easy42_session", Value: sessionToken})

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	var result engine.ApplyResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func init() {
	applyCmd.Flags().StringVarP(&applyFilePath, "file", "f", "", "Path to config change file (JSON or YAML)")
	applyCmd.Flags().StringVar(&applyName, "name", "", "Name of saved config change snippet in config.json to apply")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Preview planned changes without writing to config")
	applyCmd.Flags().BoolVarP(&applySync, "sync", "s", false, "Automatically trigger network sync after apply")
	applyCmd.Flags().StringVar(&applyFormat, "format", "text", "Output format for preview (text or json)")
	RootCmd.AddCommand(applyCmd)
}
