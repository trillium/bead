package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Decision mode: work through the items waiting on the captain (label
// captain-decision) across the brain stores, answering each as a comment.
const (
	decisionLabel = "captain-decision"
	answeredLabel = "decision-answered"
	closeReason   = "Answered by the captain via bead decide"
	fetchWorkers  = 8
)

// decision is one captain-decision item, tagged with the store it lives in.
type decision struct {
	Store        string
	ID           string
	Title        string
	Description  string
	Priority     int
	CreatedAt    string
	Labels       []string
	CommentCount int
}

type comment struct {
	Author    string
	Text      string
	CreatedAt string
}

// decisionOption is one lettered/numbered/bulleted choice listed under an
// item's OPTIONS heading. Key is the quick-pick key ("" = no quick pick).
type decisionOption struct {
	Key         string
	Label       string // explicit "A" / "2" from the item, "" for bullets
	Name        string
	Detail      string
	Recommended bool
}

// decideOutcome is what saving an answer does to the item besides adding the
// comment. Closing is only ever an explicit choice.
type decideOutcome int

const (
	outcomeAnswered decideOutcome = iota // add the decision-answered label
	outcomeComment                       // comment only
	outcomeClose                         // close the item
	outcomeCount
)

func (o decideOutcome) String() string {
	switch o {
	case outcomeComment:
		return "comment only"
	case outcomeClose:
		return "close the item"
	default:
		return "mark answered"
	}
}

// step is one store command a save runs; Stdin is "" when it takes none.
type step struct {
	Argv  []string
	Stdin string
}

func (d decision) answered() bool {
	for _, l := range d.Labels {
		if l == answeredLabel {
			return true
		}
	}
	return false
}

// jsonArray cuts the JSON array out of command output; the host may print
// notices before it.
func jsonArray(data []byte) ([]byte, error) {
	start := bytes.IndexByte(data, '[')
	if start < 0 {
		return nil, fmt.Errorf("no JSON array in output")
	}
	return data[start:], nil
}

// parseDecisions decodes `<store> list --json` output.
func parseDecisions(storeName string, data []byte) ([]decision, error) {
	arr, err := jsonArray(data)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID           string   `json:"id"`
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		Priority     int      `json:"priority"`
		CreatedAt    string   `json:"created_at"`
		Labels       []string `json:"labels"`
		CommentCount int      `json:"comment_count"`
	}
	if err := json.NewDecoder(bytes.NewReader(arr)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("parsing %s list: %w", storeName, err)
	}
	var out []decision
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		out = append(out, decision{
			Store: storeName, ID: r.ID, Title: r.Title, Description: r.Description,
			Priority: r.Priority, CreatedAt: r.CreatedAt, Labels: r.Labels,
			CommentCount: r.CommentCount,
		})
	}
	return out, nil
}

// parseComments decodes `<store> comments <id> --json` output.
func parseComments(data []byte) ([]comment, error) {
	arr, err := jsonArray(data)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Author    string `json:"author"`
		Text      string `json:"text"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(bytes.NewReader(arr)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("parsing comments: %w", err)
	}
	out := make([]comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, comment{Author: r.Author, Text: r.Text, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// waitingDecisions drops items already marked answered (unless all) and
// orders the rest: highest priority first (P0 on top), newest first within a
// priority.
func waitingDecisions(ds []decision, all bool) []decision {
	var out []decision
	for _, d := range ds {
		if all || !d.answered() {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.Priority != y.Priority {
			return x.Priority < y.Priority
		}
		if x.CreatedAt != y.CreatedAt {
			return x.CreatedAt > y.CreatedAt
		}
		if x.Store != y.Store {
			return x.Store < y.Store
		}
		return x.ID < y.ID
	})
	return out
}

// --- reading the store -------------------------------------------------

func listArgv() []string {
	return []string{"list", "-l", decisionLabel, "--json", "-n", "0"}
}

func commentsArgv(id string) []string { return []string{"comments", id, "--json"} }

// runStoreJSON runs a store wrapper and returns its stdout (stderr carries
// notices and is dropped).
func runStoreJSON(storeName string, argv []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, storeName, argv...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s %s failed: %s", storeName, argv[0], lastLine(string(ee.Stderr), err))
		}
		return nil, fmt.Errorf("%s %s: %w", storeName, argv[0], err)
	}
	return out, nil
}

// fetchDecisions lists the captain-decision items in each named store in
// parallel. Stores that fail are reported in warnings, not fatal.
func fetchDecisions(names []string) (all []decision, warnings []string) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, fetchWorkers)
	for _, name := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func(name string) {
			defer wg.Done()
			defer func() { <-sem }()
			out, err := runStoreJSON(name, listArgv())
			var ds []decision
			if err == nil {
				ds, err = parseDecisions(name, out)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
				return
			}
			all = append(all, ds...)
		}(name)
	}
	wg.Wait()
	sort.Strings(warnings)
	return all, warnings
}

// decideStoreNames picks which stores to read: the one named, or every
// registry store whose wrapper is on PATH.
func decideStoreNames(storeArg string, stores []store) ([]string, error) {
	if storeArg != "" {
		if _, err := exec.LookPath(storeArg); err != nil {
			return nil, fmt.Errorf("no such store command: %s (is its wrapper on PATH?)", storeArg)
		}
		return []string{storeArg}, nil
	}
	var names []string
	for _, s := range stores {
		if _, err := exec.LookPath(s.Name); err == nil {
			names = append(names, s.Name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no store commands found on PATH")
	}
	return names, nil
}

// --- saving an answer --------------------------------------------------

// answerComment stamps the captain's answer so it reads as one in the
// comment thread.
func answerComment(text string, now time.Time) string {
	return fmt.Sprintf("Answer (captain, %s): %s", now.Format("2006-01-02"), strings.TrimSpace(text))
}

// saveSteps lists the store commands that record an answer, comment first.
// The comment text goes in on stdin (`-f /dev/stdin`), not as a local file
// path: on a brain remote client the command runs on the host, which cannot
// see this machine's files, but the forwarder relays stdin.
func saveSteps(d decision, outcome decideOutcome, text string, now time.Time) []step {
	steps := []step{{
		Argv:  []string{"comments", "add", d.ID, "-f", "/dev/stdin"},
		Stdin: answerComment(text, now) + "\n",
	}}
	switch outcome {
	case outcomeAnswered:
		steps = append(steps, step{Argv: []string{"label", "add", d.ID, answeredLabel}})
	case outcomeClose:
		steps = append(steps, step{Argv: []string{"close", d.ID, "--reason", closeReason}})
	}
	return steps
}

// runSteps runs the steps in order against the store, stopping at the first
// failure. done is how many succeeded, so a caller can tell "comment saved,
// label failed" from "nothing saved".
func runSteps(storeName string, steps []step) (done int, err error) {
	for _, s := range steps {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, storeName, s.Argv...)
		if s.Stdin != "" {
			cmd.Stdin = strings.NewReader(s.Stdin)
		}
		out, runErr := cmd.CombinedOutput()
		cancel()
		if runErr != nil {
			return done, fmt.Errorf("%s %s failed: %s", storeName, s.Argv[0], lastLine(string(out), runErr))
		}
		done++
	}
	return done, nil
}

func lastLine(out string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return fallback.Error()
}

// formatStep renders a step as the shell-ish line a dry run reports.
func formatStep(storeName string, s step) string {
	line := storeName + " " + strings.Join(s.Argv, " ")
	if s.Stdin != "" {
		line += "   # stdin: " + strings.TrimSpace(s.Stdin)
	}
	return line
}

// --- options and links in the item text --------------------------------

var (
	headingRe = regexp.MustCompile(`^[A-Z][A-Z0-9 /&'’,.\-]{2,}(?:\s*\([^)]*\))?:?$`)
	bulletRe  = regexp.MustCompile(`^[-*•]\s+(.*)$`)
	labelRe   = regexp.MustCompile(`^\(?([A-Za-z]|\d{1,2})[.):]\s+(.*)$`)
	optionRe  = regexp.MustCompile(`(?i)^option\s+([A-Za-z]|\d{1,2})\s*[.):\-—]\s*(.*)$`)
	linkRe    = regexp.MustCompile(`https?://[^\s<>)\]"']+`)
)

func isHeading(line string) bool {
	return headingRe.MatchString(strings.TrimSpace(line))
}

func isOptionsHeading(line string) bool {
	h := strings.ToUpper(strings.TrimSpace(line))
	if i := strings.Index(h, "("); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(h), ":"))
	return h == "OPTIONS" || h == "OPTION" || h == "CHOICES" || h == "CHOICE" || h == "YOUR OPTIONS"
}

// parseOptions finds the options a decision item lists: bullet / lettered /
// numbered lines under an OPTIONS (or CHOICES) heading, else `Option A: …`
// lines anywhere. Nothing found means no quick picks.
func parseOptions(desc string) []decisionOption {
	lines := strings.Split(strings.ReplaceAll(desc, "\r\n", "\n"), "\n")
	var opts []decisionOption
	in := false
	sawSection := false
	add := func(label, text string) {
		name, detail := text, ""
		if i := strings.Index(text, ": "); i > 0 {
			name, detail = text[:i], strings.TrimSpace(text[i+2:])
		}
		o := decisionOption{Label: label, Detail: detail}
		low := strings.ToLower(name)
		if i := strings.Index(low, "(recommended)"); i >= 0 {
			o.Recommended = true
			name = name[:i] + name[i+len("(recommended)"):]
		}
		o.Name = strings.TrimSpace(strings.Join(strings.Fields(name), " "))
		if o.Name != "" {
			opts = append(opts, o)
		}
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if isHeading(line) || isOptionsHeading(line) {
			in = isOptionsHeading(line)
			sawSection = sawSection || in
			continue
		}
		if !in || line == "" {
			continue
		}
		text := line
		label := ""
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			text = m[1]
			if lm := labelRe.FindStringSubmatch(text); lm != nil {
				label, text = lm[1], lm[2]
			}
		} else if lm := labelRe.FindStringSubmatch(line); lm != nil {
			label, text = lm[1], lm[2]
		} else if n := len(opts); n > 0 && (raw != line) {
			// Indented continuation of the previous option.
			opts[n-1].Detail = strings.TrimSpace(opts[n-1].Detail + " " + line)
			continue
		} else {
			continue
		}
		add(label, text)
	}
	if !sawSection {
		opts = nil
		for _, raw := range lines {
			if m := optionRe.FindStringSubmatch(strings.TrimSpace(raw)); m != nil {
				add(m[1], m[2])
			}
		}
	}
	assignOptionKeys(opts)
	return opts
}

// assignOptionKeys gives each option its quick-pick key: the item's own
// letters/numbers when every option has one, else 1..9 by position.
func assignOptionKeys(opts []decisionOption) {
	labeled := len(opts) > 0
	seen := map[string]bool{}
	for _, o := range opts {
		k := strings.ToLower(o.Label)
		if o.Label == "" || seen[k] {
			labeled = false
		}
		seen[k] = true
	}
	for i := range opts {
		switch {
		case labeled:
			opts[i].Key = strings.ToLower(opts[i].Label)
		case i < 9:
			opts[i].Key = fmt.Sprint(i + 1)
		}
	}
}

// optionAnswer is the answer text a quick pick fills in.
func optionAnswer(o decisionOption) string {
	if o.Label != "" {
		return fmt.Sprintf("Option %s: %s", strings.ToUpper(o.Label), o.Name)
	}
	return o.Name
}

// itemLinks returns the distinct URLs in the item text, in order.
func itemLinks(desc string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range linkRe.FindAllString(desc, -1) {
		l = strings.TrimRight(l, ".,;:")
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// --- command entry -----------------------------------------------------

func printDecideHelp() {
	fmt.Println(`bead decide — answer the decisions waiting on you (label ` + decisionLabel + `).

Usage:
  bead decide [store]             work through open decisions, one answer at a time
  bead decide --list [store]      plain list of the open decisions; no TUI
  bead decide --all …             include items already marked ` + answeredLabel + `
  bead -n|--dry-run decide [store]  walk through, but run no writes (prints what would run)

Without a store, every brain store on PATH is read. Items come highest
priority first (P0 on top), newest first within a priority.

List keys:
  ↑↓       move            ⏎ open the item          esc quits

Item keys:
  tab/↑↓   move between Pick, Answer, After saving, Save
  ←/→      Pick: previous/next option (or press its number/letter)
             ⏎ on Pick uses the highlighted option
             After saving: mark answered / comment only / close the item
  ctrl+s    save the answer                    ⏎ on Save saves too
  pgup/pgdn or alt+↑/↓   scroll the item text
  ctrl+n/p  skip to the next/previous item without answering
  esc       back to the list                   ctrl+c quits

Saving adds a comment "Answer (captain, <date>): …" to the item, then marks
it ` + answeredLabel + ` so it leaves your queue and firstmate can act on it and
close it. Nothing is closed unless you pick "close the item", and an answer
is always required. The comment text goes to the store on stdin.`)
}

func runDecide(args []string, dryRun bool) int {
	listOnly, includeAnswered := false, false
	storeArg := ""
	for _, a := range args {
		switch a {
		case "-h", "--help", "help":
			printDecideHelp()
			return 0
		case "--list", "-l":
			listOnly = true
		case "--all":
			includeAnswered = true
		default:
			if looksLikeFlag(a) || storeArg != "" {
				fmt.Fprintf(os.Stderr, "bead: decide: unexpected argument %q (see `bead decide --help`)\n", a)
				return 2
			}
			storeArg = a
		}
	}

	var stores []store
	if storeArg == "" {
		var err error
		if stores, err = loadStores(); err != nil {
			fmt.Fprintf(os.Stderr, "bead: %v\n", err)
			return 1
		}
	}
	names, err := decideStoreNames(storeArg, stores)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead: %v\n", err)
		return 2
	}

	tty := isTTY(os.Stdin) && isTTY(os.Stdout)
	if tty && !listOnly {
		fmt.Fprintf(os.Stderr, "bead: reading %s items from %d store(s)…\n", decisionLabel, len(names))
	}
	found, warnings := fetchDecisions(names)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "bead: warning: %s\n", shorten(w, 140))
	}
	items := waitingDecisions(found, includeAnswered)
	if len(items) == 0 {
		if len(warnings) == len(names) {
			fmt.Fprintf(os.Stderr, "bead: no store could be read.\n")
			return 1
		}
		fmt.Fprintf(os.Stderr, "bead: no decisions waiting on you.\n")
		return 0
	}

	if listOnly || !tty {
		if !listOnly {
			fmt.Fprintf(os.Stderr, "bead: answering needs a terminal; showing the list instead.\n")
		}
		printDecisions(items)
		return 0
	}
	return runDecideTUI(items, dryRun)
}

func printDecisions(items []decision) {
	th := activeTheme()
	maxStore, maxID := 0, 0
	for _, d := range items {
		maxStore = max(maxStore, len(d.Store))
		maxID = max(maxID, len(d.ID))
	}
	for _, d := range items {
		mark := " "
		if d.answered() {
			mark = "✓"
		}
		fmt.Printf("%s %s  %s  %s  %s\n", mark,
			th.Dim.Render(padRight(d.Store, maxStore)),
			th.Name.Render(padRight(d.ID, maxID)),
			th.Title.Render(fmt.Sprintf("P%d", d.Priority)),
			d.Title)
	}
}
