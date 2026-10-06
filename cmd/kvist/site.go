package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/klppl/kvist/internal/auth"
	"github.com/klppl/kvist/internal/store"
)

func cmdSite(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: kvist site list|delete")
	}
	sub, args := args[0], args[1:]
	fset := flag.NewFlagSet("site "+sub, flag.ExitOnError)
	yes := fset.Bool("yes", false, "delete without asking (delete)")
	cfg, err := loadConfig(fset, args)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	toks := auth.Open(cfg.DataDir)
	switch sub {
	case "list":
		stored, err := st.SiteIDs()
		if err != nil {
			return err
		}
		tokens, err := toks.List()
		if err != nil {
			return err
		}
		ntok := map[string]int{}
		for _, t := range tokens {
			ntok[t.Site]++
		}
		ids := []string{}
		seen := map[string]bool{}
		for _, s := range cfg.Sites {
			ids = append(ids, s.ID)
			seen[s.ID] = true
		}
		for _, id := range stored {
			if !seen[id] {
				ids = append(ids, id)
			}
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SITE\tSTATUS\tDATA\tTOKENS")
		for _, id := range ids {
			status := "configured"
			if cfg.Site(id) == nil {
				status = "not in config"
			}
			size := "-"
			if n, ok := dirSize(filepath.Join(st.Dir(), "sites", id)); ok {
				size = formatBytes(n)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", id, status, size, ntok[id])
		}
		return w.Flush()
	case "delete":
		if fset.NArg() != 1 {
			return errors.New("usage: kvist site delete [--config FILE] [--yes] SITE")
		}
		id := fset.Arg(0)
		if cfg.Site(id) != nil {
			return fmt.Errorf("site %q is still in the config; remove its [[site]] block, restart kvist (docker compose restart kvist), then delete it", id)
		}
		size, hasData := dirSize(filepath.Join(st.Dir(), "sites", id))
		tokens, err := toks.List()
		if err != nil {
			return err
		}
		ntok := 0
		for _, t := range tokens {
			if t.Site == id {
				ntok++
			}
		}
		if !hasData && ntok == 0 {
			return fmt.Errorf("nothing is stored for site %q", id)
		}
		fmt.Printf("This deletes site %q: %s of notes, revisions and builds, and %d token(s). It cannot be undone.\n", id, formatBytes(size), ntok)
		if !*yes {
			fmt.Printf("Type the site id to confirm: ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if strings.TrimSpace(line) != id {
				return errors.New("not confirmed; nothing was deleted")
			}
		}
		if hasData {
			if err := st.RemoveSite(id); err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		revoked, err := toks.RevokeSite(id)
		if err != nil {
			return err
		}
		fmt.Printf("Deleted site %q and revoked %d token(s).\n", id, len(revoked))
	default:
		return fmt.Errorf("unknown site command %q", sub)
	}
	return nil
}

// dirSize sums the sizes of the regular files under dir; ok is false if dir
// does not exist.
func dirSize(dir string) (n int64, ok bool) {
	if _, err := os.Stat(dir); err != nil {
		return 0, false
	}
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n, true
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
