package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"easy42/internal/config"
	"easy42/internal/engine"

	"github.com/spf13/cobra"
)

var (
	nodeName  string
	nodeHost  string
	nodeIP    string
	nodeIface string
	nodeASN   uint64
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Manage network mesh nodes",
}

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.NewStore(GetDataDir())
		cfg, err := store.Load()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tHOST\tIP\tINTERFACE\tASN\tENTRYPOINTS")
		for _, n := range cfg.Nodes {
			epCount := len(n.Entrypoints)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d configured\n", n.Name, n.Host, n.IP, n.Interface, n.ASN, epCount)
		}
		w.Flush()
		return nil
	},
}

var nodeAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new node to the mesh",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.NewStore(GetDataDir())
		if _, err := store.Load(); err != nil {
			return err
		}
		mgr := engine.NewManager(store)

		node := config.Node{
			Name:        nodeName,
			Host:        nodeHost,
			IP:          nodeIP,
			Interface:   nodeIface,
			ASN:         nodeASN,
			Entrypoints: make([]config.Entrypoint, 0),
		}

		if node.Interface == "" {
			node.Interface = "lo"
		}

		if err := mgr.AddNode(node); err != nil {
			return err
		}

		fmt.Printf("Successfully added node %s (%s, IP: %s)\n", node.Name, node.Host, node.IP)
		return nil
	},
}

var nodeProbeCmd = &cobra.Command{
	Use:   "probe [host]",
	Short: "Probe a remote host over SSH to inspect network configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := args[0]
		store := config.NewStore(GetDataDir())
		_, _ = store.Load()
		mgr := engine.NewManager(store)

		fmt.Printf("Probing host %s via SSH...\n", host)
		res, err := mgr.ProbeHost(host)
		if err != nil {
			return fmt.Errorf("probe failed: %w", err)
		}

		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
		return nil
	},
}

var nodeRemoveCmd = &cobra.Command{
	Use:     "remove [name]",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a node from the mesh",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		store := config.NewStore(GetDataDir())
		if _, err := store.Load(); err != nil {
			return err
		}
		mgr := engine.NewManager(store)

		if err := mgr.DeleteNode(name); err != nil {
			return err
		}

		fmt.Printf("Node %s removed successfully\n", name)
		return nil
	},
}

var nodeBirdCmd = &cobra.Command{
	Use:   "bird [name]",
	Short: "Generate BIRD BGP routing configuration for a node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		store := config.NewStore(GetDataDir())
		if _, err := store.Load(); err != nil {
			return err
		}
		mgr := engine.NewManager(store)
		birdConf, err := mgr.GenerateBirdConfig(name)
		if err != nil {
			return err
		}
		fmt.Print(birdConf)
		return nil
	},
}

func init() {
	nodeAddCmd.Flags().StringVarP(&nodeName, "name", "n", "", "Node name (max 11 chars)")
	nodeAddCmd.Flags().StringVarP(&nodeHost, "host", "H", "", "SSH host or alias")
	nodeAddCmd.Flags().StringVarP(&nodeIP, "ip", "i", "", "Main IPv4 address")
	nodeAddCmd.Flags().StringVar(&nodeIface, "iface", "lo", "Main IP interface name")
	nodeAddCmd.Flags().Uint64VarP(&nodeASN, "asn", "a", 4224420001, "AS number")
	_ = nodeAddCmd.MarkFlagRequired("name")
	_ = nodeAddCmd.MarkFlagRequired("host")
	_ = nodeAddCmd.MarkFlagRequired("ip")

	nodeCmd.AddCommand(nodeListCmd)
	nodeCmd.AddCommand(nodeAddCmd)
	nodeCmd.AddCommand(nodeProbeCmd)
	nodeCmd.AddCommand(nodeRemoveCmd)
	nodeCmd.AddCommand(nodeBirdCmd)
	RootCmd.AddCommand(nodeCmd)
}
