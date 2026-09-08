package main

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Theme bundles the semantic styles for bead's TUIs. Palettes follow the
// published Rosé Pine (main + Moon) and Nord specs. Select with:
//
//	BEAD_THEME=rose-pine | rose-pine-moon (moon) | nord   (default rose-pine)
type Theme struct {
	name    string
	Title   lipgloss.Style // headers, field names
	Accent  lipgloss.Style // ❯ markers, focus cues
	Name    lipgloss.Style // store names, priority value
	Dim     lipgloss.Style // descriptions, hints, counts
	Hint    lipgloss.Style // tab-direction hints
	Danger  lipgloss.Style // errors
	Reverse lipgloss.Style // highlighted row / chip
}

type palette struct {
	text, muted  string
	accent, name string
	hint, danger string
}

var palettes = map[string]palette{
	"rose-pine": {
		text: "#e0def4", muted: "#6e6a86",
		accent: "#31748f", name: "#f6c177",
		hint: "#ebbcba", danger: "#eb6f92",
	},
	"rose-pine-moon": {
		text: "#e0def4", muted: "#6e6a86",
		accent: "#3e8fb0", name: "#f6c177",
		hint: "#ea9a97", danger: "#eb6f92",
	},
	"nord": {
		text: "#eceff4", muted: "#4c566a",
		accent: "#88c0d0", name: "#ebcb8b",
		hint: "#bf616a", danger: "#bf616a",
	},
}

// themeName resolves BEAD_THEME; unknown values fall back to the
// default rather than erroring.
func themeName() string {
	switch n := strings.ToLower(strings.TrimSpace(os.Getenv("BEAD_THEME"))); n {
	case "moon", "rose-pine-moon":
		return "rose-pine-moon"
	case "nord":
		return "nord"
	default:
		return "rose-pine"
	}
}

func buildTheme(name string) *Theme {
	p, ok := palettes[name]
	if !ok {
		p = palettes["rose-pine"]
		name = "rose-pine"
	}
	var r *lipgloss.Renderer
	if colorOn {
		r = lipgloss.DefaultRenderer()
	} else {
		// Piped output, NO_COLOR, or tests: plain text, no codes.
		r = lipgloss.NewRenderer(os.Stdout, termenv.WithProfile(termenv.Ascii))
	}
	return &Theme{
		name:    name,
		Title:   r.NewStyle().Bold(true).Foreground(lipgloss.Color(p.text)),
		Accent:  r.NewStyle().Bold(true).Foreground(lipgloss.Color(p.accent)),
		Name:    r.NewStyle().Bold(true).Foreground(lipgloss.Color(p.name)),
		Dim:     r.NewStyle().Foreground(lipgloss.Color(p.muted)),
		Hint:    r.NewStyle().Foreground(lipgloss.Color(p.hint)),
		Danger:  r.NewStyle().Bold(true).Foreground(lipgloss.Color(p.danger)),
		Reverse: r.NewStyle().Reverse(true),
	}
}

var appTheme *Theme

func activeTheme() *Theme {
	if appTheme == nil {
		appTheme = buildTheme(themeName())
	}
	return appTheme
}
