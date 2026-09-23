package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runBeadPi jumps into a bead-scoped pi session: the user pastes messy
// content, pi scopes it down to 1+ beads, creates them, and the session
// closes on exit.
//
//	bead pi ["initial thought..."]   interactive pi with bead scope
//	echo body | bead pi ["title"]    stdin becomes part of the initial prompt
//	bead -n pi [...]                 dry run: pi -p plans, creates nothing
func runBeadPi(args []string, dryRun bool) int {
	for _, a := range args {
		switch a {
		case "-h", "--help", "help":
			printBeadPiHelp()
			return 0
		}
	}

	piBin, err := exec.LookPath("pi")
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead pi: pi not found on PATH (%v)\n", err)
		return 1
	}

	// Initial text = argv remainder + piped stdin (if any).
	initial := strings.Join(args, " ")
	if !isTTY(os.Stdin) {
		body, err := readAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bead pi: reading stdin: %v\n", err)
			return 1
		}
		if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
			if initial == "" {
				initial = trimmed
			} else {
				initial += "\n\n" + trimmed
			}
		}
	}

	stores, err := loadStores()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead pi: %v\n", err)
		return 1
	}
	var sb strings.Builder
	for _, s := range stores {
		if s.About != "" {
			fmt.Fprintf(&sb, "  %s — %s\n", s.Name, shorten(s.About, 90))
		} else {
			fmt.Fprintf(&sb, "  %s\n", s.Name)
		}
	}

	scope := `You are bead-pi, a bead-creation specialist for the PAI beads federation (one bd binary, many named stores; each store is its own command: inbox, brain, task, ideas, ...).

STORES (name — what goes there):
` + sb.String() + `
WORKFLOW:
1. The user pastes messy content. Scope it down with them: what is it, which store(s), one bead or several?
2. Defaults: inbox = unprocessed capture queue; task = actionable work; brain = knowledge/notes; ideas = not-yet-committed possibilities. Ask when ambiguous, but don't stall on trivia.
3. Propose the plan (store + one-line title + 1-2 line body sketch per bead) and confirm briefly before creating.
4. Create each bead with: <store> create "Title" -t task -p 2 --body-file /tmp/bead-pi-body-N.md --silent
   Write the full body to the temp file first (heredoc), then create, then rm the temp file. Long content ALWAYS goes through --body-file, never on the command line.
5. Labels: list candidates with <store> label list-all when useful; skip when unsure.
6. End by printing: CREATED: <id1> <id2> ...`

	// All pi launches run from the federation root so repo-local
	// AGENTS.md/skills don't leak into the session; fall back to $HOME.
	launchDir := ""
	if home, herr := os.UserHomeDir(); herr == nil {
		launchDir = home
		if st, serr := os.Stat(home + "/data"); serr == nil && st.IsDir() {
			launchDir = home + "/data"
		}
	}

	if dryRun {
		cmd := exec.Command(piBin, "-p",
			"--append-system-prompt", scope+"\n\nDRY RUN — do NOT run any <store> create commands. Only output the JSON plan.",
			"Plan beads for:\n"+initial)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if launchDir != "" {
			cmd.Dir = launchDir
		}
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			fmt.Fprintf(os.Stderr, "bead pi: running pi: %v\n", err)
			return 1
		}
		return 0
	}

	argv := []string{"--append-system-prompt", scope}
	if strings.TrimSpace(initial) != "" {
		argv = append(argv, initial)
	}
	cmd := exec.Command(piBin, argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if launchDir != "" {
		cmd.Dir = launchDir
	}
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bead pi: running pi: %v\n", err)
		return 1
	}
	return 0
}

func printBeadPiHelp() {
	fmt.Println(`bead pi — jump into a bead-scoped pi session, paste content, scope it down, create, exit.

Usage:
  bead pi ["initial thought..."]   interactive pi; paste content, confirm plan, beads get created
  echo "body" | bead pi ["title"]  stdin becomes part of the initial prompt
  bead -n pi [...]                 dry run: pi plans (JSON), creates nothing

In the session pi knows every store, writes long bodies via --body-file,
and prints CREATED: <ids> at the end. Quit pi (/exit, ctrl-c) to close down.`)
}
