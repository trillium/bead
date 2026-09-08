package main

import (
	"strings"
	"testing"
)

func TestThemeName(t *testing.T) {
	t.Setenv("BEAD_THEME", "")
	if got := themeName(); got != "rose-pine" {
		t.Errorf("empty = %s, want rose-pine", got)
	}
	for theme, variants := range map[string][]string{
		"rose-pine":      {"rose-pine", "ROSE-PINE", "  rose-pine  ", "nope"},
		"rose-pine-moon": {"moon", "rose-pine-moon", "Moon"},
		"nord":           {"nord", "NORD"},
	} {
		for _, v := range variants {
			t.Setenv("BEAD_THEME", v)
			if got := themeName(); got != theme {
				t.Errorf("BEAD_THEME=%q = %s, want %s", v, got, theme)
			}
		}
	}
}

func TestBuildThemeFallback(t *testing.T) {
	th := buildTheme("does-not-exist")
	if th.name != "rose-pine" {
		t.Errorf("unknown palette = %s, want rose-pine fallback", th.name)
	}
	for name := range palettes {
		if buildTheme(name).name != name {
			t.Errorf("palette %s did not build as itself", name)
		}
	}
}

func TestThemeRendersPlainWithoutTTY(t *testing.T) {
	// Tests run without a tty, so styles must degrade to plain text —
	// existing view assertions on raw substrings keep working.
	th := buildTheme("nord")
	styles := []struct {
		label  string
		render func(...string) string
	}{
		{"Title", th.Title.Render}, {"Accent", th.Accent.Render},
		{"Name", th.Name.Render}, {"Dim", th.Dim.Render},
		{"Hint", th.Hint.Render}, {"Danger", th.Danger.Render},
		{"Reverse", th.Reverse.Render},
	}
	for _, st := range styles {
		if got := st.render("probe"); got != "probe" {
			t.Errorf("%s rendered %q with codes in test env", st.label, got)
		}
	}
	if !strings.Contains(palettes["nord"].hint, "bf616a") {
		t.Errorf("nord hint should be nord11 red")
	}
}
