package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/whyyor/tuimessenger/internal/integration"
	"github.com/whyyor/tuimessenger/skills"
)

var integrationUsage = `Usage: tuimessenger integration install|uninstall|status [-force] AGENT...

Installs the tuimessenger skill, so the agent knows how to read and send your chats with ` + "`tuimessenger agent`" + `.
AGENT is one of: ` + strings.Join(integration.Names(), ", ") + `. tuimessenger --skill prints the skill.`

// runIntegration is `tuimessenger integration …`.
func runIntegration(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, integrationUsage)
		return 2
	}
	fs := flag.NewFlagSet("tuimessenger integration", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("force", false, "replace a tuimessenger skill that wasn't installed by tuimessenger")
	targets, err := parseAnywhere(fs, args[1:])
	if err != nil || len(targets) == 0 {
		fmt.Fprintln(os.Stderr, integrationUsage)
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	code := 0
	for _, target := range targets {
		switch args[0] {
		case "install":
			path, err := integration.Install(home, target, skills.Tuimessenger, *force)
			switch {
			case errors.Is(err, integration.ErrNotOurs):
				fmt.Fprintf(os.Stderr, "%s: %s has a tuimessenger skill you made yourself; keep it, or replace it with -force\n", target, path)
				code = 1
			case err != nil:
				fmt.Fprintf(os.Stderr, "%s: %v\n", target, err)
				code = 1
			default:
				fmt.Printf("%s: installed %s\n", target, path)
				if target != "codex" {
					fmt.Printf("  Start a new %s session to pick it up.\n", target)
				}
			}
		case "uninstall":
			removed, err := integration.Uninstall(home, target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", target, err)
				code = 1
			}
			for _, path := range removed {
				fmt.Printf("%s: removed %s\n", target, path)
			}
			if len(removed) == 0 && err == nil {
				fmt.Printf("%s: nothing installed by tuimessenger\n", target)
			}
		case "status":
			found := integration.Status(home, target, skills.Tuimessenger)
			if len(found) == 0 {
				fmt.Printf("%s: not installed\n", target)
			}
			for _, f := range found {
				state := "installed, up to date"
				switch {
				case !f.Ours:
					state = "your own tuimessenger skill"
				case !f.UpToDate:
					state = "installed, outdated: run tuimessenger integration install " + target
				}
				fmt.Printf("%s: %s (%s)\n", target, f.Path, state)
			}
		default:
			fmt.Fprintln(os.Stderr, integrationUsage)
			return 2
		}
	}
	return code
}
