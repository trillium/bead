package main

import (
	"sort"
	"strings"
)

// matchTier ranks how well q matches: exact name > name prefix >
// name substring > about substring. Lower is better; -1 = no match.
func matchTier(name, about, q string) int {
	name = strings.ToLower(name)
	switch {
	case name == q:
		return 0
	case strings.HasPrefix(name, q):
		return 1
	case strings.Contains(name, q):
		return 2
	case strings.Contains(strings.ToLower(about), q):
		return 3
	default:
		return -1
	}
}

// filterStores returns store indices matching q (case-insensitive substring
// over name and about). Empty query matches everything, preserving order.
func filterStores(stores []store, q string) []int {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		all := make([]int, len(stores))
		for i := range stores {
			all[i] = i
		}
		return all
	}
	var out []int
	for i, s := range stores {
		if matchTier(s.Name, s.About, q) >= 0 {
			out = append(out, i)
		}
	}
	// Best match first; ties keep registry order (stable).
	sort.SliceStable(out, func(a, b int) bool {
		return matchTier(stores[out[a]].Name, stores[out[a]].About, q) <
			matchTier(stores[out[b]].Name, stores[out[b]].About, q)
	})
	return out
}

// visibleRows is the list height: capped, shrunk for short terminals, and
// never more than the matches. Always >= 1 so the empty state still renders.
func visibleRows(termHeight, n int) int {
	rows := maxPickerRows
	if termHeight > 0 {
		if avail := termHeight - 6; avail < rows {
			rows = avail
		}
	}
	if rows > n {
		rows = n
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func dropLastWord(q string) string {
	trimmed := strings.TrimRight(q, " ")
	if i := strings.LastIndex(trimmed, " "); i >= 0 {
		return trimmed[:i+1]
	}
	return ""
}
