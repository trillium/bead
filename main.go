// bead — human entry wizard for beads federation stores.
//
// Bare `bead` opens a store picker, then an in-app entry form; `<store>
// create` receives the long content via --body-file, never over the
// command line.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type store struct {
	Name  string
	Path  string
	About string
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	colorOn = isTTY(os.Stdout) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	dryRun := false
	positionals := make([]string, 0, len(argv))
	for _, a := range argv {
		switch a {
		case "-n", "--dry-run":
			dryRun = true
		default:
			positionals = append(positionals, a)
		}
	}

	// Help / list work without a store.
	if len(positionals) > 0 {
		switch positionals[0] {
		case "-h", "--help", "help":
			printHelp()
			return 0
		case "--list", "stores":
			stores, err := loadStores()
			if err != nil {
				fmt.Fprintf(os.Stderr, "bead: %v\n", err)
				return 1
			}
			printStores(stores)
			return 0
		}
	}

	// bead pi: jump into a bead-scoped pi session (before store routing).
	if len(positionals) > 0 && positionals[0] == "pi" {
		return runBeadPi(positionals[1:], dryRun)
	}

	storeName := ""
	prefillTitle := ""
	if len(positionals) > 0 {
		storeName = positionals[0]
	}
	if len(positionals) > 1 {
		prefillTitle = positionals[1]
	}
	// Ignore extras beyond title, like the bash original.

	stores, err := loadStores()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead: %v\n", err)
		return 1
	}
	byName := map[string]store{}
	for _, s := range stores {
		byName[s.Name] = s
		byName[strings.ToLower(s.Name)] = s
	}

	fullTUI := isTTY(os.Stdin) && isTTY(os.Stdout)

	if storeName == "" {
		// Bare `bead` → picker. `bead -n` → picker in dry-run mode.
		if !isTTY(os.Stdin) {
			printUsage()
			fmt.Fprintf(os.Stderr, "\nbead: a store is required for piped input: bead <store> \"Title\" < body\n")
			return 2
		}
		if !fullTUI {
			// No TUI possible (stdout piped): pick now, enter via editor.
			picked, ok := pickStore(stores)
			if !ok {
				fmt.Fprintf(os.Stderr, "bead: aborted (no store selected).\n")
				return 1
			}
			storeName = picked.Name
		}
		// Else: runInteractive picks.
	} else if _, known := byName[storeName]; !known {
		if _, lookErr := exec.LookPath(storeName); lookErr != nil {
			// Friendly path: `bead "Some title"` (store omitted) drops into
			// the picker with the title prefilled instead of dying on
			// "no such store command".
			if isTTY(os.Stdin) && prefillTitle == "" && !looksLikeFlag(storeName) {
				if fullTUI {
					prefillTitle = storeName
					storeName = ""
				} else {
					picked, ok := pickStore(stores)
					if !ok {
						fmt.Fprintf(os.Stderr, "bead: aborted (no store selected).\n")
						return 1
					}
					prefillTitle = storeName
					storeName = picked.Name
				}
			} else {
				fmt.Fprintf(os.Stderr, "bead: no such store command: %s (is its wrapper on PATH?)\n", storeName)
				fmt.Fprintf(os.Stderr, "bead: run `bead` with no args to pick from all stores.\n")
				return 2
			}
		}
	}

	// Empty storeName is fine when the TUI will pick (bare `bead` on a full
	// terminal); runInteractive validates after picking.
	if storeName != "" {
		if _, lookErr := exec.LookPath(storeName); lookErr != nil {
			fmt.Fprintf(os.Stderr, "bead: no such store command: %s (is its wrapper on PATH?)\n", storeName)
			return 2
		}
	}

	// Non-interactive: body piped on stdin.
	if !isTTY(os.Stdin) {
		if prefillTitle == "" {
			fmt.Fprintf(os.Stderr, "bead: a title is required:  bead %s \"Title\" < body\n", storeName)
			return 2
		}
		body, err := readAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bead: reading stdin: %v\n", err)
			return 1
		}
		args := []string{"create", prefillTitle}
		var bodyFile string
		if len(bytesTrimSpace(body)) > 0 {
			bodyFile, err = writeTemp("bead-body.*", body)
			if err != nil {
				fmt.Fprintf(os.Stderr, "bead: %v\n", err)
				return 1
			}
			defer os.Remove(bodyFile)
			args = append(args, "--body-file", bodyFile)
		}
		if dryRun {
			args = append(args, "--dry-run")
		}
		return execStore(storeName, args)
	}

	// Interactive (stdin is a tty here): full TUI entry when stdout is also
	// a tty, else the legacy editor form.
	if fullTUI {
		return runInteractive(storeName, prefillTitle, dryRun, stores)
	}
	return runEditorForm(storeName, prefillTitle, dryRun)
}

func printHelp() {
	fmt.Println(`bead — human entry wizard for beads federation stores.

Usage:
  bead                            pick a store, then fill the entry form → create
  bead <store>                    entry form for that store → create
  bead <store> "Short title"      entry form with the title prefilled
  bead "Short title"              pick a store with the title prefilled
  bead pi ["thought..."]          bead-scoped pi session: paste, scope down, create, exit
  echo "body" | bead <store> "T"  non-interactive: title + piped body
  bead -n|--dry-run <store> …     preview; create nothing
  bead --list                     list known stores and exit

Entry form keys:
  tab/↑↓   move between Title, Priority, Labels, Description
  ←/→      Priority down/up (or type 0-4)      ⏎ in Labels adds the match
  ctrl+s    create the bead                     esc backs out / quits

Flags (any position):
  -n, --dry-run   forward --dry-run to the store's create (previews, no write).

Long content goes through --body-file (the description), never over the
command line, so quoting/length limits can't mangle it.`)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "usage: bead [store] [\"title\"]\n")
	if stores, err := loadStores(); err == nil {
		fmt.Fprintf(os.Stderr, "stores:\n")
		for _, s := range stores {
			fmt.Fprintf(os.Stderr, "  %s\n", s.Name)
		}
	}
	fmt.Fprintf(os.Stderr, "run `bead --help` for details, or `bead` with no args to pick interactively.\n")
}
