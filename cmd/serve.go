package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"easy42/internal/config"
	"easy42/internal/engine"
	"easy42/internal/server"
	"easy42/web"
	"github.com/spf13/cobra"
)

type listenAddrValue struct {
	addrs   []string
	changed bool
}

func (v *listenAddrValue) String() string {
	if len(v.addrs) == 0 {
		return "127.0.0.1:4242"
	}
	return strings.Join(v.addrs, ",")
}

func (v *listenAddrValue) Set(val string) error {
	if !v.changed {
		v.addrs = nil
		v.changed = true
	}
	for _, part := range strings.Split(val, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			v.addrs = append(v.addrs, trimmed)
		}
	}
	listenAddr = strings.Join(v.addrs, ",")
	return nil
}

func (v *listenAddrValue) Type() string {
	return "string"
}

var (
	listenAddr  string
	listenValue = listenAddrValue{
		addrs: []string{"127.0.0.1:4242"},
	}
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start easy42 backend and web UI server",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.NewStore(GetDataDir())

		// First-time boot initialization
		if !store.Exists() {
			fmt.Fprintf(os.Stderr, "=== easy42 First-Time Setup ===\n")
			fmt.Fprintf(os.Stderr, "Config not found at %s. Initializing...\n", GetDataDir())
			rawPassword, err := store.Initialize()
			if err != nil {
				return fmt.Errorf("initialization failed: %w", err)
			}
			fmt.Fprintf(os.Stderr, "=======================================================\n")
			fmt.Fprintf(os.Stderr, "Generated Web UI Admin Password:  %s\n", rawPassword)
			fmt.Fprintf(os.Stderr, "Save this password! You will need it to login to the Web UI.\n")
			fmt.Fprintf(os.Stderr, "=======================================================\n\n")
		} else {
			if _, err := store.Load(); err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
		}

		mgr := engine.NewManager(store)
		mgr.StartROABackgroundRefresher(context.Background(), 1*time.Hour)
		distFS, err := web.DistFS()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: unable to load embedded frontend: %v\n", err)
		}

		if len(listenValue.addrs) == 0 {
			return fmt.Errorf("no listen addresses specified")
		}

		srv := server.New(server.Config{
			ListenAddrs: listenValue.addrs,
			ListenAddr:  listenAddr,
			Manager:     mgr,
			DistFS:      distFS,
		})

		for _, addr := range srv.ListenAddrs() {
			fmt.Printf("easy42 web server starting on http://%s\n", addr)
		}
		fmt.Printf("Data directory: %s\n", GetDataDir())

		primaryAddr := "127.0.0.1:4242"
		addrs := srv.ListenAddrs()
		if len(addrs) > 0 {
			primaryAddr = addrs[0]
			host, port, err := net.SplitHostPort(primaryAddr)
			if err == nil && (host == "0.0.0.0" || host == "" || host == "::") {
				primaryAddr = net.JoinHostPort("127.0.0.1", port)
			}
		}
		serverURLFile := filepath.Join(GetDataDir(), "server.url")
		_ = os.WriteFile(serverURLFile, []byte(fmt.Sprintf("http://%s", primaryAddr)), 0644)
		defer os.Remove(serverURLFile)

		// Graceful shutdown handling
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-sigChan
			fmt.Println("\nShutting down easy42 server...")
			_ = os.Remove(serverURLFile)
			_ = srv.Shutdown(cmd.Context())
			os.Exit(0)
		}()

		return srv.Start()
	},
}

func init() {
	listenAddr = "127.0.0.1:4242"
	serveCmd.Flags().VarP(&listenValue, "listen", "l", "Address to listen on")
	RootCmd.AddCommand(serveCmd)
}
