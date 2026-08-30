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

	"queueup/internal/a2s"
	"queueup/internal/relay"
	"queueup/internal/servers"
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
                                     (token only: to sign in on the website,
                                     register there instead)
  relay delete-account <email>       remove an account that has no PCs or joins
  relay query <ip:port>              ask a Rust server how it is doing, right
                                     now, the same way the wipe watcher does
  relay set-subscription <email> <active|none>
                                     open or close the gate for one account by
                                     hand (used for testing and comped accounts
                                     until Stripe does this automatically)

Settings come from environment variables, never from files in the repo:

  QUEUEUP_ADDR          address to listen on          (default :8080)
  QUEUEUP_DB            database file                 (default queueup.db)
  QUEUEUP_ADMIN_TOKEN   token for /admin/status       (admin view is off if unset)

  QUEUEUP_SERVER_SOURCE where server search comes from (default stub)
                          stub           a built-in list. No key, works offline.
                          steam          Steam's server list. Needs a FREE key in
                                         QUEUEUP_STEAM_API_KEY, from
                                         https://steamcommunity.com/dev/apikey
                          battlemetrics  Needs a PAID subscription token in
                                         QUEUEUP_BATTLEMETRICS_TOKEN

  QUEUEUP_BILLING=on     turn the subscription gate on. Off by default, which
                         means every account runs free. Flip it when Stripe is
                         connected.
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
	case "query":
		if len(args) < 2 {
			return errors.New("usage: relay query <ip:port>")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		info, err := a2s.Query(ctx, args[1], 3*time.Second)
		cancel()
		if err != nil {
			return fmt.Errorf("that server did not answer: %w", err)
		}
		fmt.Printf("  name:     %s\n  map:      %s\n  players:  %d / %d\n  queue:    %d\n  keywords: %s\n",
			info.Name, info.Map, info.Players, info.MaxPlayers, info.Queue, info.Keywords)
		return nil
	case "delete-account":
		if len(args) < 2 {
			return errors.New("usage: relay delete-account <email>")
		}
		acct, err := st.AccountByEmail(args[1])
		if err != nil {
			return fmt.Errorf("no account for %s", args[1])
		}
		if err := st.DeleteAccount(acct.ID); err != nil {
			return err
		}
		fmt.Printf("Deleted %s.\n", acct.Email)
		return nil
	case "set-subscription":
		if len(args) < 3 {
			return errors.New("usage: relay set-subscription <email> <active|none>")
		}
		acct, err := st.AccountByEmail(args[1])
		if err != nil {
			return fmt.Errorf("no account for %s", args[1])
		}
		if err := st.SetSubscription(acct.ID, args[2], ""); err != nil {
			return err
		}
		fmt.Printf("Done. %s is now %q.\n", acct.Email, args[2])
		return nil
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

	provider, err := servers.FromEnv()
	if err != nil {
		return err
	}
	if provider.Name() == "stub" {
		log.Warn("server search is using the built-in stub list. " +
			"Set QUEUEUP_SERVER_SOURCE to steam or battlemetrics for real servers")
	}

	billing := os.Getenv("QUEUEUP_BILLING") == "on"
	if !billing {
		log.Warn("billing is off: every account runs free. Set QUEUEUP_BILLING=on once Stripe is connected")
	}

	srv := relay.New(relay.Config{
		Store: st, Log: log, AdminToken: adminToken, Servers: provider,
		BillingEnabled: billing,
	})
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv,
		// No write timeout: agent connections and status streams are meant to
		// stay open for hours.
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The scheduler fires planned joins; the watcher polls each active job's
	// server and is what detects a wipe restart.
	go srv.RunScheduler(ctx, 5*time.Second)
	watcher := &relay.Watcher{Store: st, Hub: srv.Hub(), Log: log}
	go watcher.Run(ctx)

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
