package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"text/tabwriter"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	linkPassword string
	fromPort     int
	toPort       int
	linkIface    string
	linkType     string
	fromIface    string
	toIface      string
	fromAddr     string
	toAddr       string
)

var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Manage mesh and peering links between nodes",
}

var linkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured links",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.NewStore(GetDataDir())
		cfg, err := store.Load()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "TYPE\tFROM NODE\tINTERFACE\tADDRESS\tPORT\tTO NODE\tINTERFACE\tADDRESS\tPORT")
		for _, l := range cfg.Links {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%d\n",
				l.LinkType(),
				l.From.Name, l.From.Interface, l.From.Address, l.From.ListenPort,
				l.To.Name, l.To.Interface, l.To.Address, l.To.ListenPort)
		}
		w.Flush()
		return nil
	},
}

var linkAddCmd = &cobra.Command{
	Use:   "add [node1] [node2]",
	Short: "Add a new link between two nodes (wireguard or manual)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		node1, node2 := args[0], args[1]
		store := config.NewStore(GetDataDir())
		if _, err := store.Load(); err != nil {
			return err
		}
		mgr := engine.NewManager(store)

		if strings.EqualFold(linkType, config.LinkTypeManual) {
			if fromIface == "" || toIface == "" || fromAddr == "" || toAddr == "" {
				return fmt.Errorf("manual link requires --from-iface, --to-iface, --from-addr, and --to-addr")
			}
			fromEnd := &config.LinkEnd{
				Name:            node1,
				Type:            config.LinkTypeManual,
				Interface:       fromIface,
				Address:         fromAddr,
				NeighborAddress: toAddr,
			}
			toEnd := &config.LinkEnd{
				Name:            node2,
				Type:            config.LinkTypeManual,
				Interface:       toIface,
				Address:         toAddr,
				NeighborAddress: fromAddr,
			}
			link, err := mgr.AddLinkAdvanced(node1, node2, fromEnd, toEnd, nil)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully created manual link between %s and %s\n", link.From.Name, link.To.Name)
			fmt.Printf("  %s: iface=%s, addr=%s, neighbor=%s\n", link.From.Name, link.From.Interface, link.From.Address, link.From.NeighborAddress)
			fmt.Printf("  %s: iface=%s, addr=%s, neighbor=%s\n", link.To.Name, link.To.Interface, link.To.Address, link.To.NeighborAddress)
			return nil
		}

		// Password needed to encrypt private keys for wireguard link
		pass := linkPassword
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

		if err := mgr.Unlock(pass); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		link, err := mgr.AddLink(node1, node2, fromPort, toPort, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Successfully created WireGuard link between %s and %s\n", link.From.Name, link.To.Name)
		fmt.Printf("  %s: iface=%s, addr=%s, port=%d\n", link.From.Name, link.From.Interface, link.From.Address, link.From.ListenPort)
		fmt.Printf("  %s: iface=%s, addr=%s, port=%d\n", link.To.Name, link.To.Interface, link.To.Address, link.To.ListenPort)
		return nil
	},
}

var linkRemoveCmd = &cobra.Command{
	Use:     "remove [node1] [node2]",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a link between two nodes",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		node1, node2 := args[0], args[1]
		store := config.NewStore(GetDataDir())
		if _, err := store.Load(); err != nil {
			return err
		}
		mgr := engine.NewManager(store)

		if err := mgr.DeleteLink(node1, node2, linkIface); err != nil {
			return err
		}

		if linkIface != "" {
			fmt.Printf("Link (%s) between %s and %s removed successfully\n", linkIface, node1, node2)
		} else {
			fmt.Printf("Link between %s and %s removed successfully\n", node1, node2)
		}
		return nil
	},
}

func init() {
	linkAddCmd.Flags().StringVarP(&linkType, "type", "t", "wireguard", "Link type ('wireguard' or 'manual')")
	linkAddCmd.Flags().StringVarP(&linkPassword, "password", "p", "", "easy42 password (for wireguard links)")
	linkAddCmd.Flags().IntVar(&fromPort, "from-port", 0, "Custom listen port for node1")
	linkAddCmd.Flags().IntVar(&toPort, "to-port", 0, "Custom listen port for node2")
	linkAddCmd.Flags().StringVar(&fromIface, "from-iface", "", "Tunnel interface name on node1 (required for manual link)")
	linkAddCmd.Flags().StringVar(&toIface, "to-iface", "", "Tunnel interface name on node2 (required for manual link)")
	linkAddCmd.Flags().StringVar(&fromAddr, "from-addr", "", "Local IP on node1 (required for manual link)")
	linkAddCmd.Flags().StringVar(&toAddr, "to-addr", "", "Local IP on node2 (required for manual link)")
	linkRemoveCmd.Flags().StringVarP(&linkIface, "interface", "i", "", "Specific interface name of the link to remove")

	linkCmd.AddCommand(linkListCmd)
	linkCmd.AddCommand(linkAddCmd)
	linkCmd.AddCommand(linkRemoveCmd)
	RootCmd.AddCommand(linkCmd)
}
