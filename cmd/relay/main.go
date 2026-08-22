// Command relay is the QueueUp relay server.
//
// It holds every agent's connection open, stores jobs durably, and gives the web
// app somewhere to send commands. Agents dial out to it, so it is the only piece
// that needs to be reachable from the internet.
//
//	go run ./cmd/relay create-account you@example.com
//	go run ./cmd/relay serve
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"queueup/internal/relay"
	"queueup/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() string {
	return strings.TrimSpace(`
QueueUp relay

  relay serve                        start the relay
  relay create-account <email>       create an account and print its token

Settings come from environment variables, never from files in the repo:

  QUEUEUP_ADDR          address to listen on          (default :8080)
  QUEUEUP_DB            database file                 (default queueup.db)
  QUEUEUP_ADMIN_TOKEN   token for /admin/status       (admin view is off if unset)
`)
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Println(usage())
		return nil
	}

	dbPath := envOr("QUEUEUP_DB", "queueup.db")
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	switch args[0] {
	case "serve":
		return serve(st)
	case "create-account":
		if len(args) < 2 {
			return errors.New("usage: relay create-account <email>")
		}
		return createAccount(st, args[1])
	default:
		fmt.Println(usage())
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func createAccount(st *store.Store, email string) error {
	if existing, err := st.AccountByEmail(email); err == nil {
		return fmt.Errorf("an account for %s already exists (%s). Its token was shown when it was created and cannot be shown again", email, existing.ID)
	}
	acct, token, err := st.CreateAccount(email)
	if err != nil {
		return err
	}
	fmt.Printf("Account created.\n\n  email:      %s\n  account id: %s\n  token:      %s\n\n",
		acct.Email, acct.ID, token)
	fmt.Println("Save that token somewhere safe. It is not stored in a readable form")
	fmt.Println("and cannot be shown again.")
	return nil
}

func serve(st *store.Store) error {
	addr := envOr("QUEUEUP_ADDR", ":8080")
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	adminToken := os.Getenv("QUEUEUP_ADMIN_TOKEN")
	if adminToken == "" {
		log.Warn("QUEUEUP_ADMIN_TOKEN is not set, so /admin/status is switched off")
	}

	srv := relay.New(relay.Config{Store: st, Log: log, AdminToken: adminToken})
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv,
		// No write timeout: agent connections and status streams are meant to
		// stay open for hours.
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("relay listening", "addr", addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
