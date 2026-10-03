package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/pushclient"
)

func cmdPush(args []string) error {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	dir := fs.String("dir", ".", "vault folder")
	server := fs.String("server", os.Getenv("KVIST_SERVER"), "server URL (or KVIST_SERVER)")
	site := fs.String("site", os.Getenv("KVIST_SITE"), "site id (or KVIST_SITE)")
	device := fs.String("device", "", "device name shown on the server (default: host name)")
	force := fs.Bool("force", false, "commit even if it would restore older content (after checking!)")
	noWait := fs.Bool("no-wait", false, "don't wait for the build to finish")
	verbose := fs.Bool("v", false, "list the publish decision for every note")
	if err := fs.Parse(args); err != nil {
		return err
	}
	token := os.Getenv("KVIST_TOKEN")
	if *server == "" || *site == "" || token == "" {
		return errors.New("--server, --site and the KVIST_TOKEN environment variable are required")
	}
	c, err := pushclient.New(*server, *site, token)
	if err != nil {
		return err
	}
	pushclient.ClientVersion = version
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	start := time.Now()
	res, err := pushclient.Push(ctx, c, pushclient.Options{
		Dir:        *dir,
		Force:      *force,
		Wait:       !*noWait,
		DeviceName: *device,
		Progress: func(done, total int) {
			fmt.Fprintf(os.Stderr, "\ruploading %d/%d", done, total)
			if done == total {
				fmt.Fprintln(os.Stderr)
			}
		},
	})
	var se *pushclient.StaleError
	if errors.As(err, &se) {
		fmt.Fprintln(os.Stderr, "The server has newer content than this folder:")
		for _, r := range se.Regressions {
			switch r.Kind {
			case protocol.RegressionOlder:
				fmt.Fprintf(os.Stderr, "  older    %s (server: %s, here: %s)\n", r.Path, r.StoredMTime.Local().Format(time.DateTime), r.IncomingMTime.Local().Format(time.DateTime))
			default:
				fmt.Fprintf(os.Stderr, "  removed  %s (changed on the server %s)\n", r.Path, r.StoredMTime.Local().Format(time.DateTime))
			}
		}
		return errors.New(se.Message + " (re-run with --force if this folder is right)")
	}
	if err != nil {
		return err
	}

	if *verbose {
		for _, d := range res.Scan.Decisions {
			mark := "-"
			if d.Published {
				mark = "+"
			}
			fmt.Printf("%s %-18s %s\n", mark, d.Reason, d.Path)
		}
	}
	for _, l := range res.Scan.UnpublishedLinks {
		kind := "links to"
		if l.Embed {
			kind = "embeds"
		}
		fmt.Printf("note: %s %s %s, which is not published (shown as plain text)\n", l.From, kind, l.Target)
	}
	notes := 0
	for _, f := range res.Scan.Files {
		if protocol.IsNote(f.Path) {
			notes++
		}
	}
	if res.Unchanged {
		fmt.Printf("Up to date with %s: %d notes, %d other files\n", res.Revision, notes, len(res.Scan.Files)-notes)
		for _, w := range res.Warnings {
			printWarning(w)
		}
		return nil
	}
	fmt.Printf("Pushed %s: %d notes, %d other files, %d uploaded (%s)\n",
		res.Revision, notes, len(res.Scan.Files)-notes, res.Uploaded, time.Since(start).Round(time.Millisecond))
	for _, w := range res.Warnings {
		printWarning(w)
	}
	if res.Build != nil {
		fmt.Printf("Build %s: %s\n", res.Build.ID, res.Build.State)
		for _, w := range res.Build.Warnings {
			printWarning(w)
		}
		if res.Build.State == protocol.BuildFailed {
			return fmt.Errorf("build failed: %s", res.Build.Error)
		}
	}
	return nil
}

func printWarning(w protocol.Warning) {
	if w.Path != "" {
		fmt.Printf("warning [%s] %s: %s\n", w.Code, w.Path, w.Message)
	} else {
		fmt.Printf("warning [%s] %s\n", w.Code, w.Message)
	}
}
