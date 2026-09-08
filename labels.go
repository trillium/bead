package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// labelsMsg delivers store labels fetched in the background (see Init).
// Labels load async so the form renders instantly; suggestions appear
// when ready. allLabels nil = still loading.
type labelsMsg []string

func fetchLabelsCmd(storeName string) tea.Cmd {
	return func() tea.Msg {
		// Warm cache first: instant suggestions on repeat visits.
		if labels, ok := readLabelCache(cacheDir(), storeName); ok {
			return labelsMsg(labels)
		}
		labels := loadStoreLabels(storeName)
		if len(labels) > 0 {
			// Best-effort; a failed write just means slow next time.
			_ = writeLabelCache(cacheDir(), storeName, labels)
		}
		return labelsMsg(labels)
	}
}

// labelCacheTTL balances freshness against a 2–16s fetch: labels are
// low-churn, and free-text entry always works, so 5 minutes is safe.
// Older caches still render instantly (stale-while-revalidate) with a
// background refresh; only a missing cache shows the loading state.
const labelCacheTTL = 5 * time.Minute

type labelCacheFile struct {
	FetchedAt int64    `json:"fetched_at"`
	Labels    []string `json:"labels"`
}

func cacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		return ""
	}
	return dir + "/bead"
}

func cacheFile(dir, storeName string) string {
	var b strings.Builder
	for _, r := range storeName {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return dir + "/labels-" + b.String() + ".json"
}

func readLabelCache(dir, storeName string) ([]string, bool) {
	labels, age, found := readLabelCacheFull(dir, storeName)
	if !found || age > labelCacheTTL {
		return nil, false
	}
	return labels, true
}

// readLabelCacheFull returns cached labels with their age, regardless of
// freshness — the stale-while-revalidate path renders these instantly.
func readLabelCacheFull(dir, storeName string) (labels []string, age time.Duration, found bool) {
	if dir == "" {
		return nil, 0, false
	}
	data, err := os.ReadFile(cacheFile(dir, storeName))
	if err != nil {
		return nil, 0, false
	}
	var cf labelCacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, 0, false
	}
	return cf.Labels, time.Since(time.Unix(cf.FetchedAt, 0)), true
}

func writeLabelCache(dir, storeName string, labels []string) error {
	if dir == "" {
		return fmt.Errorf("no cache dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(labelCacheFile{FetchedAt: time.Now().Unix(), Labels: labels})
	if err != nil {
		return err
	}
	// Atomic write: temp + rename, so readers never see partial JSON.
	tmp, err := os.CreateTemp(dir, ".labels-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, cacheFile(dir, storeName))
}

// matchLabels filters labels by case-insensitive substring, excluding
// chosen, best match first (exact > prefix > substring, ties keep order).
func matchLabels(all []string, selected []string, q string) []string {
	chosen := map[string]bool{}
	for _, s := range selected {
		chosen[strings.ToLower(s)] = true
	}
	q = strings.ToLower(strings.TrimSpace(q))
	type cand struct {
		label string
		tier  int
	}
	var cands []cand
	for _, l := range all {
		if chosen[strings.ToLower(l)] {
			continue
		}
		if t := matchTier(l, "", q); t >= 0 {
			cands = append(cands, cand{l, t})
		}
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].tier < cands[b].tier })
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.label
	}
	return out
}

// loadStoreLabels returns existing label names for fuzzy search via
// `<store> label list-all`, most-used first (stable within equal counts).
// Frequency order survives the tier sorts downstream, so one-off hashes
// and notify: IDs sink below labels actually in use. Empty (not fatal)
// when unavailable.
func loadStoreLabels(storeName string) []string {
	out, err := exec.Command(storeName, "label", "list-all").Output()
	if err != nil {
		return nil
	}
	counts := parseLabelList(string(out))
	labels := make([]string, len(counts))
	for i, c := range counts {
		labels[i] = c.name
	}
	return labels
}

type labelCount struct {
	name  string
	count int
}

// parseLabelList parses `label list-all` output (`  <name>  (N issues)`
// per line) into most-used-first order. Malformed lines are skipped.
func parseLabelList(out string) []labelCount {
	var counts []labelCount
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "All labels") || strings.HasPrefix(trimmed, "🏷") {
			continue
		}
		name, count := splitLabelCount(trimmed)
		if name != "" && !strings.HasPrefix(name, "(") {
			counts = append(counts, labelCount{name, count})
		}
	}
	sort.SliceStable(counts, func(a, b int) bool { return counts[a].count > counts[b].count })
	return counts
}

// splitLabelCount cuts the trailing `(N issue[s])` count off a list-all
// line; lines without one count as 1 (still a real label).
func splitLabelCount(line string) (string, int) {
	if i := strings.LastIndex(line, "("); i > 0 {
		tail := strings.TrimSpace(line[i:])
		var n int
		if _, err := fmt.Sscanf(tail, "(%d issue", &n); err == nil {
			return strings.TrimSpace(line[:i]), n
		}
	}
	return line, 1
}
