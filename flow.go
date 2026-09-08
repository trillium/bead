package main

import (
	"fmt"
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

// runCreate executes `<store> create …` with the body via --body-file.
// Returns the store's exit code.
func runCreate(storeName string, args []string, body string, dryRun bool) int {
	if strings.TrimSpace(body) != "" {
		bodyFile, err := writeTemp("bead-body.*", []byte(strings.TrimRight(body, "\n")+"\n"))
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
	rc := execStore(storeName, args)
	if dryRun {
		fmt.Fprintf(os.Stderr, "bead: dry run — nothing created.\n")
	}
	return rc
}
