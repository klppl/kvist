package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/klppl/kvist/internal/model"
	"github.com/klppl/kvist/internal/source/dir"
)

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	site := fs.String("site", "", "site id from the config (default: the only site)")
	vault := fs.String("dir", "", "vault folder to build from (development)")
	emit := fs.String("emit-model", "", "write the content model as JSON to this file (- for stdout)")
	cfg, err := loadConfig(fs, args)
	if err != nil {
		return err
	}
	if *vault == "" || *emit == "" {
		return errors.New("usage: kvist build --dir VAULT --emit-model FILE [--site ID] (HTML output arrives in Phase 3)")
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
	m, err := model.Build(sc, snap)
	if err != nil {
		return err
	}
	for _, w := range m.Warnings {
		printWarning(w)
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
