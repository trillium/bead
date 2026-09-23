package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Default bead-pi theme: rose-pine-moon reads as "bead session" at a glance
// and stays distinct from stock pi's dark default. Overridable per launch
// with --theme. If the named theme is not installed, pi falls back to dark.
const beadPiDefaultTheme = "rose-pine-moon"

// Narrow mode launches pi with only what bead creation needs: the bash and
// read tools, no skills, no context files. Extension tools stay discoverable
// but pi's --tools allowlist applies to built-in, extension, and custom tools
// alike, so nothing outside the allowlist can run.
const beadPiNarrowTools = "bash,read"

type beadPiOptions struct {
	narrow bool
	theme  string // "" means the bead-pi default tint
}

// runBeadPi jumps into a bead-scoped pi session: the user pastes messy
// content, pi scopes it down to 1+ beads, creates them, and the session
// closes on exit.
//
//	bead pi ["initial thought..."]   interactive pi with bead scope
//	bead pi --narrow ["thought..."]  stripped-down pi (minimal tools/skills)
//	bead pi --theme <name> [...]     tint the session (default: rose-pine-moon)
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

	opts, rest, err := parseBeadPiArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead pi: %v\n", err)
		return 2
	}

	piBin, err := exec.LookPath("pi")
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead pi: pi not found on PATH (%v)\n", err)
		return 1
	}

	// Initial text = argv remainder + piped stdin (if any).
	initial := strings.Join(rest, " ")
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

	scope := beadPiScope(stores, opts.narrow)

	// All pi launches run from the federation root so repo-local
	// AGENTS.md/skills don't leak into the session; fall back to $HOME.
	launchDir := ""
	if home, herr := os.UserHomeDir(); herr == nil {
		launchDir = home
		if st, serr := os.Stat(home + "/data"); serr == nil && st.IsDir() {
			launchDir = home + "/data"
		}
	}

	var argv []string
	if dryRun {
		argv = buildBeadPiDryRunArgv(opts, scope, initial)
	} else {
		argv = buildBeadPiArgv(opts, scope, initial)
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

// parseBeadPiArgs pulls bead-pi flags out of args; everything else is
// initial prompt text (preserved verbatim, in order).
func parseBeadPiArgs(args []string) (beadPiOptions, []string, error) {
	var opts beadPiOptions
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--narrow":
			opts.narrow = true
		case a == "--theme" || a == "-theme":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--theme needs a theme name (e.g. --theme rose-pine-moon)")
			}
			i++
			opts.theme = args[i]
		case strings.HasPrefix(a, "--theme="):
			opts.theme = strings.TrimPrefix(a, "--theme=")
			if opts.theme == "" {
				return opts, nil, fmt.Errorf("--theme needs a theme name (e.g. --theme rose-pine-moon)")
			}
		default:
			rest = append(rest, a)
		}
	}
	return opts, rest, nil
}

// beadPiTheme resolves the --use-theme value: explicit --theme wins,
// otherwise the bead-pi default tint.
func beadPiTheme(opts beadPiOptions) string {
	if opts.theme != "" {
		return opts.theme
	}
	return beadPiDefaultTheme
}

// buildBeadPiArgv assembles the interactive pi invocation. Pure function so
// the flag wiring is unit-testable without launching pi.
func buildBeadPiArgv(opts beadPiOptions, scope, initial string) []string {
	argv := []string{"--use-theme", beadPiTheme(opts)}
	if opts.narrow {
		argv = append(argv, "--no-skills", "--no-context-files", "--tools", beadPiNarrowTools)
	}
	argv = append(argv, "--append-system-prompt", scope)
	if strings.TrimSpace(initial) != "" {
		argv = append(argv, initial)
	}
	return argv
}

// buildBeadPiDryRunArgv assembles the non-interactive planner invocation
// (pi -p). Narrow restrictions apply here too so the plan reflects what the
// stripped-down session can actually do.
func buildBeadPiDryRunArgv(opts beadPiOptions, scope, initial string) []string {
	argv := []string{"-p", "--use-theme", beadPiTheme(opts)}
	if opts.narrow {
		argv = append(argv, "--no-skills", "--no-context-files", "--tools", beadPiNarrowTools)
	}
	argv = append(argv,
		"--append-system-prompt", scope+"\n\nDRY RUN — do NOT run any <store> create commands. Only output the JSON plan.",
		"Plan beads for:\n"+initial)
	return argv
}

// beadPiScope builds the system prompt. Full mode stays additive on purpose:
// the global AGENTS.md (project memory) already teaches the federation model
// — thin-wrapper store commands, per-store prefixes, --body-file creation —
// so the prompt only needs the live store list plus the workflow. Narrow mode
// passes --no-context-files, so AGENTS.md never loads and the prompt must
// carry the FULL runbook itself to stay self-sufficient.
func beadPiScope(stores []store, narrow bool) string {
	var sb strings.Builder
	for _, s := range stores {
		if s.About != "" {
			fmt.Fprintf(&sb, "  %s — %s\n", s.Name, shorten(s.About, 90))
		} else {
			fmt.Fprintf(&sb, "  %s\n", s.Name)
		}
	}

	if narrow {
		return `You are bead-pi (NARROW MODE), a bead-creation specialist for the PAI beads federation.
You run stripped-down on purpose: no skills, no context files, only the bash + read tools. Everything you need is in this prompt — do not go looking for docs elsewhere.

FEDERATION MODEL (thin wrappers, one bd binary, many named stores):
- Every store is its own shell command: inbox, brain, task, ideas, person, ... Each is a thin sh wrapper that pins that store's BEADS_DIR/BRAIN_KNOWLEDGE_ROOT and delegates to ` + "`bd`" + `.
- NEVER use a bare ` + "`bd`" + ` — it resolves to whatever repo's store the cwd happens to carry, not the federation. Always the store command.
- An id is self-describing: inbox-xxxx, task-xxxx, brain-xxxx. The prefix tells you the store.
- Store commands are keyed by name, not by directory: run them from your launch directory, they work from anywhere.

STORES (name — what goes there):
` + sb.String() + `
STORE ROUTING (defaults; ask when ambiguous, never stall on trivia):
- inbox = unprocessed inputs / capture queue (default landing pad)
- task = actionable work items
- brain = general knowledge, notes, patterns
- ideas = possibilities not yet committed to
- person = people, contacts, relationship context

WORKFLOW:
1. The user pastes messy content. Scope it down with them: what is it, which store(s), one bead or several?
2. Propose the plan (store + one-line title + 1-2 line body sketch + priority + type per bead) and confirm briefly before creating.
3. DUPLICATE CHECK (mandatory before each create): run <store> search "<2-4 keywords>" on the target store (or <store> find-duplicates); if an open bead already covers it, report the existing id instead of creating a duplicate.
4. Write the full markdown body to a temp file first, e.g.: cat > /tmp/bead-pi-body-1.md <<'EOF' ... EOF
5. Create each bead with: <store> create "Title" -t <type> -p <priority> --body-file /tmp/bead-pi-body-N.md --silent
   (--silent prints only the new id.) Then rm the temp file. Long content ALWAYS goes through --body-file, never on the command line.
6. Priority scale: 0-4 or P0-P4, 0 = highest urgency, default 2 (normal). P0 = drop everything, P1 = soon, P2 = normal backlog, P3/P4 = someday.
7. Type options (-t): bug | feature | task | epic | chore | decision (default task). Pick the one that fits; don't leave everything as task out of laziness.
8. Labels: list candidates with <store> label list-all when useful; skip when unsure.
9. End by printing: CREATED: <id1> <id2> ...`
	}

	return `You are bead-pi, a bead-creation specialist for the PAI beads federation (one bd binary, many named stores; each store is its own command: inbox, brain, task, ideas, ...).

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
}

func printBeadPiHelp() {
	fmt.Println(`bead pi — jump into a bead-scoped pi session, paste content, scope it down, create, exit.

Usage:
  bead pi ["initial thought..."]   interactive pi; paste content, confirm plan, beads get created
  bead pi --narrow ["thought..."]  stripped-down pi: only bash+read tools, no skills,
                                   no context files; the scope prompt carries the full runbook
  bead pi --theme <name> [...]     tint the session (default: rose-pine-moon)
  echo "body" | bead pi ["title"]  stdin becomes part of the initial prompt
  bead -n pi [...]                 dry run: pi plans (JSON), creates nothing

Themes: dark, light (pi built-ins) plus any ~/.pi/agent/themes/*.json by file
name (e.g. rose-pine-moon). A missing theme falls back to dark.

In the session pi knows every store, writes long bodies via --body-file,
and prints CREATED: <ids> at the end. Quit pi (/exit, ctrl-c) to close down.`)
}
