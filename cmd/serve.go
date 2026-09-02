package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"easy42/internal/config"
	"easy42/internal/engine"
	"easy42/internal/server"
	"easy42/web"
	"github.com/spf13/cobra"
)

var (
	listenAddr string
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
		distFS, err := web.DistFS()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: unable to load embedded frontend: %v\n", err)
		}

		srv := server.New(server.Config{
			ListenAddr: listenAddr,
			Manager:    mgr,
			DistFS:     distFS,
		})

		fmt.Printf("easy42 web server starting on http://%s\n", listenAddr)
		fmt.Printf("Data directory: %s\n", GetDataDir())

		// Graceful shutdown handling
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-sigChan
			fmt.Println("\nShutting down easy42 server...")
			_ = srv.Shutdown(cmd.Context())
			os.Exit(0)
		}()

		return srv.Start()
	},
}

func init() {
	serveCmd.Flags().StringVarP(&listenAddr, "listen", "l", "127.0.0.1:4242", "Address to listen on")
	RootCmd.AddCommand(serveCmd)
}
