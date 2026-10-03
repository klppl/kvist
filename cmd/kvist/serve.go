package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/klppl/kvist/internal/api"
	"github.com/klppl/kvist/internal/auth"
	"github.com/klppl/kvist/internal/build"
	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/serve"
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
	builder := &build.SiteBuilder{Config: cfg, Store: st, Log: log}
	q := build.NewQueue(builder, func(site string) string {
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

	apiHandler := api.New(svc, auth.Open(cfg.DataDir), version, log)
	var served []serve.Site
	for _, sc := range cfg.Sites {
		if !sc.Serve {
			continue
		}
		ss, err := st.Site(sc.ID)
		if err != nil {
			return err
		}
		served = append(served, serve.Site{Host: serve.HostOf(sc.BaseURL), PublicDir: filepath.Join(ss.Dir(), "public")})
	}
	static := serve.New(served)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || len(served) == 0 {
			apiHandler.ServeHTTP(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
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
	// GC never builds; the queue only satisfies the service.
	q := build.NewQueue(build.BuilderFunc(func(context.Context, string, string, string) ([]protocol.Warning, error) {
		return nil, nil
	}), nil, nil)
	defer q.Close()
	syncer.New(cfg, st, q, slog.New(slog.NewTextHandler(os.Stderr, nil))).GCAll()
	return nil
}
