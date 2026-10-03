package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/devserver"
)

func cmdDev(args []string) error {
	fs := flag.NewFlagSet("dev", flag.ExitOnError)
	cfgPath := fs.String("config", "", "server config to take the site's rules and theme from (optional)")
	site := fs.String("site", "", "site id in the config (default: the only site)")
	vault := fs.String("dir", ".", "vault folder")
	listen := fs.String("listen", "127.0.0.1:1313", "address to serve on")
	public := fs.String("public", "", "without --config: comma-separated folders that are always public")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cfgPath == "" {
		if _, err := os.Stat(defaultConfigPath()); err == nil {
			*cfgPath = defaultConfigPath()
		}
	}
	var sc *config.Site
	if *cfgPath != "" {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		sc = cfg.Site(*site)
		if *site == "" && len(cfg.Sites) == 1 {
			sc = cfg.Sites[0]
		}
		if sc == nil {
			return fmt.Errorf("unknown site %q in %s", *site, *cfgPath)
		}
		return runDev(sc, cfg.ThemesDir, *vault, *listen)
	}
	var folders []string
	for _, f := range strings.Split(*public, ",") {
		if f = strings.TrimSpace(f); f != "" {
			folders = append(folders, strconv.Quote(f))
		}
	}
	cfg, err := config.Parse([]byte(fmt.Sprintf(`data_dir = "unused"
[[site]]
id = "dev"
title = "kvist dev"
base_url = "http://%s"
  [site.publish]
  always_public_folders = [%s]
  expose_frontmatter = ["stage"]
`, *listen, strings.Join(folders, ", "))))
	if err != nil {
		return err
	}
	if len(folders) == 0 {
		fmt.Fprintln(os.Stderr, "No config: only notes tagged #public or with publish: true are shown (use --public FOLDER or --config).")
	}
	return runDev(cfg.Sites[0], cfg.ThemesDir, *vault, *listen)
}

func runDev(sc *config.Site, themesDir, vault, listen string) error {
	if fi, err := os.Stat(vault); err != nil || !fi.IsDir() {
		return errors.New("--dir must be a vault folder")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	s := &devserver.Server{Site: sc, ThemesDir: themesDir, Vault: vault, Log: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	return s.Run(ctx, listen)
}
