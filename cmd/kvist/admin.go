package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/klppl/kvist/internal/auth"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/store"
)

func cmdToken(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: kvist token create|list|revoke")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("token "+sub, flag.ExitOnError)
	site := fs.String("site", "", "site id (create; default: the only site)")
	name := fs.String("name", "", "a label for the token, e.g. the device (create; default: token-<date>)")
	cfg, err := loadConfig(fs, args)
	if err != nil {
		return err
	}
	toks := auth.Open(cfg.DataDir)
	switch sub {
	case "create":
		if *site == "" && len(cfg.Sites) == 1 {
			*site = cfg.Sites[0].ID
		}
		if cfg.Site(*site) == nil {
			ids := make([]string, len(cfg.Sites))
			for i, s := range cfg.Sites {
				ids[i] = s.ID
			}
			if *site == "" {
				return fmt.Errorf("this server has several sites (%s); choose one with --site", strings.Join(ids, ", "))
			}
			return fmt.Errorf("unknown site %q (sites: %s)", *site, strings.Join(ids, ", "))
		}
		if *name == "" {
			*name = "token-" + time.Now().Format("2006-01-02")
		}
		if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
			return err
		}
		t, secret, err := toks.Create(*site, *name, time.Now())
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Created token %s for site %q (%s). It is shown only once:\n", t.ID, t.Site, t.Name)
		fmt.Println(secret)
	case "list":
		list, err := toks.List()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSITE\tNAME\tSCOPE\tCREATED")
		for _, t := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.Site, t.Name, t.Scope, t.Created.Format(time.RFC3339))
		}
		return w.Flush()
	case "revoke":
		if fs.NArg() != 1 {
			return errors.New("usage: kvist token revoke [--config FILE] ID")
		}
		t, err := toks.Revoke(fs.Arg(0))
		if err != nil {
			return err
		}
		fmt.Printf("Revoked token %s (%s, %s)\n", t.ID, t.Site, t.Name)
	default:
		return fmt.Errorf("unknown token command %q", sub)
	}
	return nil
}

func cmdRollback(args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	cfg, err := loadConfig(fs, args)
	if err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: kvist rollback [--config FILE] SITE REVISION")
	}
	siteID, revID := fs.Arg(0), fs.Arg(1)
	sc := cfg.Site(siteID)
	if sc == nil {
		return fmt.Errorf("unknown site %q", siteID)
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	site, err := st.Site(siteID)
	if err != nil {
		return err
	}
	unlock, err := site.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	old, err := site.Revision(revID)
	if errors.Is(err, store.ErrNotFound) {
		ids, _ := site.Revisions()
		return fmt.Errorf("revision %s is not retained; available: %v", revID, ids)
	}
	if err != nil {
		return err
	}
	head, err := site.Head()
	if err != nil {
		return err
	}
	rev, err := site.Commit(store.CommitParams{
		ExpectedHead: head,
		Client:       protocol.Client{Name: "kvist rollback"},
		Message:      "rollback to " + old.ID,
		Files:        old.Files,
		Now:          time.Now(),
		KeepRevs:     sc.Retention.Revisions,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Committed %s with the content of %s. A running server builds it within a few seconds.\n", rev.ID, old.ID)
	fmt.Println("Note: the next push from a device replaces it again; unpublish the content in the vault too.")
	return nil
}
