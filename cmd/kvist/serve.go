package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/klppl/kvist/internal/api"
	"github.com/klppl/kvist/internal/auth"
	"github.com/klppl/kvist/internal/build"
	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/store"
	"github.com/klppl/kvist/internal/syncer"
)

func loadConfig(fs *flag.FlagSet, args []string) (*config.Config, error) {
	path := fs.String("config", defaultConfigPath(), "config file")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return config.Load(*path)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfg, err := loadConfig(fs, args)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	q := build.NewQueue(build.Placeholder, func(site string) string {
		s, err := st.Site(site)
		if err != nil {
			return os.TempDir()
		}
		return s.Dir()
	}, log)
	defer q.Close()
	svc := syncer.New(cfg, st, q, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go svc.Run(ctx, 10*time.Minute, 2*time.Second)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(svc, auth.Open(cfg.DataDir), version, log),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("kvist listening", "addr", cfg.Listen, "version", version, "sites", len(cfg.Sites))
		if cfg.TLSCert != "" {
			errc <- srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			errc <- srv.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func cmdGC(args []string) error {
	fs := flag.NewFlagSet("gc", flag.ExitOnError)
	cfg, err := loadConfig(fs, args)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	q := build.NewQueue(build.Placeholder, nil, nil)
	defer q.Close()
	syncer.New(cfg, st, q, slog.New(slog.NewTextHandler(os.Stderr, nil))).GCAll()
	return nil
}
