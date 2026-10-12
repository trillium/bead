package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// runInteractive loops store picker → entry form → create.
// direct is true when the store came from argv (`bead <store>`): esc in the
// entry form quits instead of returning to the picker.
func runInteractive(storeName, prefillTitle string, dryRun bool, stores []store) int {
	direct := storeName != ""
	first := true
	for {
		if storeName == "" {
			picked, ok := pickStore(stores)
			if !ok {
				fmt.Fprintf(os.Stderr, "bead: aborted (no store selected).\n")
				return 1
			}
			storeName = picked.Name
		}
		if _, err := exec.LookPath(storeName); err != nil {
			fmt.Fprintf(os.Stderr, "bead: no such store command: %s (is its wrapper on PATH?)\n", storeName)
			return 2
		}
		prefill := ""
		if first {
			prefill = prefillTitle
			first = false
		}
		em := newEntryModel(storeName, prefill, dryRun)
		// Stale-while-revalidate: preseed from any cache (instant file
		// read) so suggestions show immediately; Init refreshes behind.
		if labels, age, found := readLabelCacheFull(cacheDir(), storeName); found {
			em.allLabels = labels
			em.stale = age > labelCacheTTL
			em.refilterLabels()
		}
		for {
			final, res := runEntryTUI(em)
			if res == entryBack {
				if direct {
					fmt.Fprintf(os.Stderr, "bead: aborted.\n")
					return 1
				}
				storeName = ""
				break // back to the picker
			}
			rc := runCreate(storeName, final.createArgs(), final.body(), dryRun)
			if rc == 0 {
				return 0
			}
			// Failed: reopen the form with values preserved + error shown.
			final.errMsg = fmt.Sprintf("create failed (rc=%d) — see output above", rc)
			final.result = entryNone
			em = final
		}
	}
}

// createInvocation builds the argv for `<store> create` and the stdin to
// feed it. A description goes in on stdin via `--body-file -`, never as a
// local temp-file path: on a brain remote client the create runs on the
// host, which cannot see this machine's files, but the forwarder relays stdin.
// hasStdin is false when there is no body (stdin is then left untouched).
func createInvocation(args []string, body string, dryRun bool) (argv []string, stdin string, hasStdin bool) {
	argv = append([]string{}, args...)
	if strings.TrimSpace(body) != "" {
		argv = append(argv, "--body-file", "-")
		stdin = strings.TrimRight(body, "\n") + "\n"
		hasStdin = true
	}
	if dryRun {
		argv = append(argv, "--dry-run")
	}
	return argv, stdin, hasStdin
}

// execCreate runs `<store> create …` with the body on stdin and returns the
// store's exit code.
func execCreate(storeName string, args []string, body string, dryRun bool) int {
	argv, stdin, hasStdin := createInvocation(args, body, dryRun)
	var in io.Reader = os.Stdin
	if hasStdin {
		in = strings.NewReader(stdin)
	}
	return execStore(storeName, argv, in)
}

// runCreate is execCreate plus the dry-run notice.
func runCreate(storeName string, args []string, body string, dryRun bool) int {
	rc := execCreate(storeName, args, body, dryRun)
	if dryRun {
		fmt.Fprintf(os.Stderr, "bead: dry run — nothing created.\n")
	}
	return rc
}
