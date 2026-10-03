package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/klppl/kvist/internal/build"
	"github.com/klppl/kvist/internal/model"
	"github.com/klppl/kvist/internal/source/dir"
)

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	site := fs.String("site", "", "site id from the config (default: the only site)")
	vault := fs.String("dir", "", "vault folder to build from (development)")
	emit := fs.String("emit-model", "", "write the content model as JSON to this file (- for stdout)")
	out := fs.String("out", "", "write the site to this folder (replaced on success)")
	cfg, err := loadConfig(fs, args)
	if err != nil {
		return err
	}
	if *vault == "" || (*emit == "" && *out == "") {
		return errors.New("usage: kvist build --dir VAULT [--out DIR] [--emit-model FILE] [--site ID]")
	}
	sc := cfg.Site(*site)
	if *site == "" && len(cfg.Sites) == 1 {
		sc = cfg.Sites[0]
	}
	if sc == nil {
		return fmt.Errorf("unknown site %q", *site)
	}
	snap, err := dir.New(*vault).Snapshot(context.Background())
	if err != nil {
		return err
	}
	if *out != "" {
		theme, err := build.LoadTheme(cfg.ThemesDir, sc)
		if err != nil {
			return err
		}
		tmp := strings.TrimRight(*out, "/") + ".kvist-tmp"
		_ = os.RemoveAll(tmp)
		warnings, err := build.WriteSite(context.Background(), sc, theme, snap, tmp)
		if err != nil {
			_ = os.RemoveAll(tmp)
			return err
		}
		for _, w := range warnings {
			printWarning(w)
		}
		if err := os.RemoveAll(*out); err != nil {
			return err
		}
		if err := os.Rename(tmp, *out); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Wrote the site to %s\n", *out)
		if *emit == "" {
			return nil
		}
	}
	m, err := model.Build(sc, snap)
	if err != nil {
		return err
	}
	if *out == "" {
		for _, w := range m.Warnings {
			printWarning(w)
		}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if *emit == "-" {
		_, err = os.Stdout.Write(b)
		return err
	}
	if err := os.WriteFile(*emit, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Wrote %s: %d notes, %d assets, %d tags\n", *emit, len(m.Notes), len(m.Assets), len(m.AllTags))
	return nil
}
