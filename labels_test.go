package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLabelsArriveAsync(t *testing.T) {
	m := newEntryModel("task", "", false)
	if m.allLabels != nil {
		t.Fatalf("allLabels should start nil (loading), got %v", m.allLabels)
	}
	if len(m.suggest) != 0 {
		t.Fatalf("no suggestions while loading, got %v", m.suggest)
	}
	u, _ := m.Update(labelsMsg([]string{"bug", "frontend"}))
	m = u.(entryModel)
	if m.allLabels == nil || len(m.suggest) != 2 {
		t.Errorf("after labelsMsg: labels=%v suggest=%v", m.allLabels, m.suggest)
	}
	// Typing filters the late-arriving labels.
	m = update(m, "tab", "tab")
	m = typeText(m, "bug")
	if len(m.suggest) != 1 || m.suggest[0] != "bug" {
		t.Errorf("suggest = %v, want [bug]", m.suggest)
	}
}

func TestStaleFlagClearsOnRefresh(t *testing.T) {
	m := newEntryModel("task", "", false)
	m.allLabels = []string{"old"}
	m.stale = true
	m.refilterLabels()
	if !strings.Contains(m.View(), "updating") {
		t.Errorf("stale view should hint updating")
	}
	u, _ := m.Update(labelsMsg([]string{"new"}))
	m = u.(entryModel)
	if m.stale {
		t.Errorf("stale should clear on refresh")
	}
	if len(m.suggest) != 1 || m.suggest[0] != "new" {
		t.Errorf("suggest = %v, want refreshed [new]", m.suggest)
	}
}

func TestLabelCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok := readLabelCache(dir, "task"); ok {
		t.Fatalf("empty cache should miss")
	}
	if err := writeLabelCache(dir, "task", []string{"bug", "frontend"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, ok := readLabelCache(dir, "task")
	if !ok || len(got) != 2 || got[0] != "bug" {
		t.Errorf("got %v,%v want [bug frontend],true", got, ok)
	}
	// Corrupt file misses instead of erroring.
	os.WriteFile(dir+"/labels-task.json", []byte("{nope"), 0o644)
	if _, ok := readLabelCache(dir, "task"); ok {
		t.Errorf("corrupt cache should miss")
	}
}

func TestLabelCacheStale(t *testing.T) {
	dir := t.TempDir()
	old := labelCacheFile{FetchedAt: 1, Labels: []string{"bug"}}
	data, _ := json.Marshal(old)
	os.WriteFile(dir+"/labels-task.json", data, 0o644)
	if _, ok := readLabelCache(dir, "task"); ok {
		t.Errorf("1970 cache should be stale")
	}
}

func TestLabelCacheFullAge(t *testing.T) {
	dir := t.TempDir()
	if _, _, found := readLabelCacheFull(dir, "task"); found {
		t.Fatalf("missing cache should not be found")
	}
	if err := writeLabelCache(dir, "task", []string{"bug"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	labels, age, found := readLabelCacheFull(dir, "task")
	if !found || len(labels) != 1 || age < 0 || age > labelCacheTTL {
		t.Errorf("got %v,%v,%v want [bug],fresh,true", labels, age, found)
	}
	// Ancient file: still found (SWR renders it), but stale.
	old := labelCacheFile{FetchedAt: 1, Labels: []string{"old"}}
	data, _ := json.Marshal(old)
	os.WriteFile(dir+"/labels-task.json", data, 0o644)
	labels, age, found = readLabelCacheFull(dir, "task")
	if !found || labels[0] != "old" || age <= labelCacheTTL {
		t.Errorf("stale file: got %v,%v,%v want rendered-but-stale", labels, age, found)
	}
	if _, ok := readLabelCache(dir, "task"); ok {
		t.Errorf("fresh-only read should miss stale file")
	}
}

func TestStoreRankingExactFirst(t *testing.T) {
	stores := []store{
		{Name: "dump", About: "Interrupted work: mid-task context cuts"},
		{Name: "external_llm_tasks", About: "Handoff asks"},
		{Name: "nightshift-tasks", About: "Queue for jobs"},
		{Name: "task", About: "Actionable work items"},
		{Name: "workflows", About: "Agent-owned tasks queue"},
	}
	got := filterStores(stores, "task")
	want := []string{"task", "external_llm_tasks", "nightshift-tasks", "dump", "workflows"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, idx := range got {
		if stores[idx].Name != want[i] {
			t.Errorf("pos %d = %s, want %s (full: %v)", i, stores[idx].Name, want[i], got)
		}
	}
}

func TestStoreRankingPrefixBeforeSubstring(t *testing.T) {
	stores := []store{
		{Name: "nightshift-tasks", About: "x"},
		{Name: "task", About: "x"},
		{Name: "talon", About: "x"},
	}
	got := filterStores(stores, "ta")
	// talon + task are prefix matches (registry order), nightshift substring last.
	if len(got) != 3 || stores[got[0]].Name != "task" || stores[got[1]].Name != "talon" || stores[got[2]].Name != "nightshift-tasks" {
		t.Errorf("got %v, want [task talon nightshift-tasks]", got)
	}
}

func TestLabelRankingExactFirst(t *testing.T) {
	got := matchLabels([]string{"bug-fix", "debug", "bug"}, nil, "bug")
	want := []string{"bug", "bug-fix", "debug"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestParseLabelListOrdersByFrequency(t *testing.T) {
	out := "🏷 All labels (4 unique):\n" +
		"  notify:abc123          (1 issues)\n" +
		"  bug                   (21 issues)\n" +
		"  a1b2c3d4e5            (1 issues)\n" +
		"  frontend              (9 issues)\n" +
		"  weird (name)          (2 issues)\n"
	got := parseLabelList(out)
	want := []labelCount{
		{"bug", 21}, {"frontend", 9}, {"weird (name)", 2},
		{"notify:abc123", 1}, {"a1b2c3d4e5", 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d = %+v, want %+v (full: %+v)", i, got[i], want[i], got)
		}
	}
}

func TestParseLabelListSkipsJunk(t *testing.T) {
	out := "some header\n\n  (3 issues)\n  good  (2 issues)\n"
	got := parseLabelList(out)
	if len(got) != 1 || got[0].name != "good" {
		t.Errorf("got %+v, want [good]", got)
	}
}

func TestMatchLabels(t *testing.T) {
	all := []string{"Frontend", "backend", "bug"}
	got := matchLabels(all, []string{"frontend"}, "end")
	if len(got) != 1 || got[0] != "backend" {
		t.Errorf("got %v, want [backend] (case-insensitive, excludes selected)", got)
	}
}
