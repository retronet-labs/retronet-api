// Comando retronet-api: backend HTTP/WebSocket per sessioni RetroNet.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/retronet-labs/retronet-api/internal/server"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout *os.File, stderr *os.File) int {
	fs := flag.NewFlagSet("retronet-api", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:8080", "indirizzo HTTP da ascoltare")
	maxSessions := fs.Int("max-sessions", 32, "numero massimo di sessioni attive")
	ttl := fs.Duration("session-ttl", 30*time.Minute, "durata massima inattiva di una sessione")
	maxFileSize := fs.Int64("max-file-size", 64*1024, "dimensione massima file nel drive sessione")
	maxFiles := fs.Int("max-files", 64, "numero massimo file nel drive sessione")
	conformance := fs.Bool("conformance", false, "esegue un controllo sintetico locale e termina")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg := server.Config{
		Version:     version,
		MaxSessions: *maxSessions,
		SessionTTL:  *ttl,
		MaxFileSize: *maxFileSize,
		MaxFiles:    *maxFiles,
	}
	if *conformance {
		if err := server.RunConformance(context.Background(), cfg); err != nil {
			fmt.Fprintf(stderr, "conformance failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "conformance passed")
		return 0
	}

	app := server.New(cfg)
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(stdout, "retronet-api listening on http://%s\n", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(stderr, "errore server: %v\n", err)
		return 1
	}
	return 0
}
