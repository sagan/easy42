package cmd

import (
	"fmt"
	"text/tabwriter"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/spf13/cobra"
)

var (
	liveStatus bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show mesh topology nodes, links, and WireGuard status",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.NewStore(GetDataDir())
		if !store.Exists() {
			return fmt.Errorf("easy42 is not initialized")
		}

		cfg, err := store.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		mgr := engine.NewManager(store)

		fmt.Printf("=== easy42 Network Mesh Status ===\n\n")

		// Nodes Table
		fmt.Printf("NODES (%d total):\n", len(cfg.Nodes))
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tHOST\tMAIN IP\tINTERFACE\tASN\tSTATUS")
		for _, n := range cfg.Nodes {
			statusStr := "CONFIGURED"
			if liveStatus {
				st, err := mgr.RefreshNodeStatus(n.Name)
				if err != nil || !st.Connected {
					statusStr = "UNREACHABLE"
				} else {
					statusStr = fmt.Sprintf("ONLINE (%d ifaces)", len(st.WgInterfaces))
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n", n.Name, n.Host, n.IP, n.Interface, n.ASN, statusStr)
		}
		w.Flush()

		// Links Table
		fmt.Printf("\nWIREGUARD LINKS (%d total):\n", len(cfg.Links))
		w2 := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w2, "FROM NODE\tINTERFACE\tPORT\tTO NODE\tINTERFACE\tPORT\tKEEPALIVE")
		for _, l := range cfg.Links {
			fmt.Fprintf(w2, "%s\t%s\t%d\t%s\t%s\t%d\t%ds\n",
				l.From.Name, l.From.Interface, l.From.ListenPort,
				l.To.Name, l.To.Interface, l.To.ListenPort,
				l.From.PersistentKeepalive)
		}
		w2.Flush()

		if !liveStatus {
			fmt.Println("\nTip: Use 'easy42 status --live' to probe live node connectivity over SSH.")
		}

		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVarP(&liveStatus, "live", "r", false, "Query live node statuses over SSH")
	RootCmd.AddCommand(statusCmd)
}
