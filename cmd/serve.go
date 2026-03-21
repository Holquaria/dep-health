package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"dep-health/server"
	"dep-health/store"
)

var (
	flagServePort int
	flagServeDB   string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the dep-health HTTP server and dashboard",
	Long: `serve launches a REST API and a React dashboard on the given port.

The dashboard lets you trigger scans, browse historical results, and inspect
per-dependency risk scores, cascade groups, and upgrade guidance.

Environment variables:
  DEP_HEALTH_DB   Path to the SQLite database file (overridden by --db)`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntVar(&flagServePort, "port", 8080, "Port to listen on")
	serveCmd.Flags().StringVar(&flagServeDB, "db", "", "Path to SQLite database file (default: dep-health.db)")
}

func runServe(_ *cobra.Command, _ []string) error {
	dbPath := flagServeDB
	if dbPath == "" {
		if v := os.Getenv("DEP_HEALTH_DB"); v != "" {
			dbPath = v
		} else {
			dbPath = "dep-health.db"
		}
	}

	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	if err := st.RecoverStuckScans(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: recover stuck scans: %v\n", err)
	}

	srv := server.New(st)
	addr := fmt.Sprintf(":%d", flagServePort)
	fmt.Fprintf(os.Stderr, "dep-health server listening on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, srv)
}
