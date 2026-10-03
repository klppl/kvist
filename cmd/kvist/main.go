// Command kvist publishes selected notes of an Obsidian vault as a website.
package main

import (
	"fmt"
	"os"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

const usage = `kvist — publish an Obsidian vault as a digital garden

Usage:
  kvist serve    [--config kvist.toml]               run the server
  kvist push     --server URL --site ID [--dir .]    push a vault folder (token in KVIST_TOKEN)
  kvist build    --dir VAULT --out DIR               build a site from a folder (--emit-model FILE for the model)
  kvist dev      [--dir VAULT] [--config FILE]       preview a vault with live reload
  kvist token    create|list|revoke [flags]          manage push tokens
  kvist rollback [--config kvist.toml] SITE REV      make an earlier revision current again
  kvist gc       [--config kvist.toml]               remove expired syncs and unreferenced blobs
  kvist version                                      print the version

Run "kvist <command> -h" for the flags of a command.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args)
	case "push":
		err = cmdPush(args)
	case "build":
		err = cmdBuild(args)
	case "dev":
		err = cmdDev(args)
	case "token":
		err = cmdToken(args)
	case "rollback":
		err = cmdRollback(args)
	case "gc":
		err = cmdGC(args)
	case "version", "--version", "-v":
		fmt.Println("kvist", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "kvist: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "kvist:", err)
		os.Exit(1)
	}
}

func defaultConfigPath() string {
	if p := os.Getenv("KVIST_CONFIG"); p != "" {
		return p
	}
	return "kvist.toml"
}
