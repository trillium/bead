package main

import (
	"strings"
	"testing"
)

func containsFlag(argv []string, flags ...string) bool {
	set := map[string]bool{}
	for _, a := range argv {
		set[a] = true
	}
	for _, f := range flags {
		if !set[f] {
			return false
		}
	}
	return true
}

func flagValue(argv []string, flag string) (string, bool) {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag {
			return argv[i+1], true
		}
	}
	return "", false
}

func TestParseBeadPiArgsNarrow(t *testing.T) {
	opts, rest, err := parseBeadPiArgs([]string{"--narrow", "some thought"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.narrow {
		t.Error("narrow not set")
	}
	if len(rest) != 1 || rest[0] != "some thought" {
		t.Errorf("rest mangled: %q", rest)
	}
}

func TestParseBeadPiArgsTheme(t *testing.T) {
	opts, rest, err := parseBeadPiArgs([]string{"--theme", "light", "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.theme != "light" {
		t.Errorf("theme = %q, want light", opts.theme)
	}
	if len(rest) != 1 || rest[0] != "hi" {
		t.Errorf("rest mangled: %q", rest)
	}

	opts, _, err = parseBeadPiArgs([]string{"--theme=nord"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.theme != "nord" {
		t.Errorf("theme = %q, want nord", opts.theme)
	}

	if _, _, err = parseBeadPiArgs([]string{"--theme"}); err == nil {
		t.Error("expected error for bare --theme")
	}
}

func TestBuildBeadPiArgvNarrow(t *testing.T) {
	argv := buildBeadPiArgv(beadPiOptions{narrow: true}, "scope", "hello")
	if !containsFlag(argv, "--no-skills", "--no-context-files", "--tools") {
		t.Errorf("narrow argv missing lockdown flags: %q", argv)
	}
	if v, ok := flagValue(argv, "--tools"); !ok || v != "bash,read" {
		t.Errorf("--tools = %q, want bash,read: %q", v, argv)
	}
	if v, ok := flagValue(argv, "--use-theme"); !ok || v != beadPiDefaultTheme {
		t.Errorf("--use-theme = %q, want default %q: %q", v, beadPiDefaultTheme, argv)
	}
}

func TestBuildBeadPiArgvFullKeepsTools(t *testing.T) {
	argv := buildBeadPiArgv(beadPiOptions{}, "scope", "hello")
	if containsFlag(argv, "--no-skills", "--no-context-files", "--tools") {
		t.Errorf("full mode must not restrict tools/skills: %q", argv)
	}
	if v, ok := flagValue(argv, "--use-theme"); !ok || v != beadPiDefaultTheme {
		t.Errorf("--use-theme = %q, want default %q: %q", v, beadPiDefaultTheme, argv)
	}
}

func TestBuildBeadPiArgvThemeOverride(t *testing.T) {
	argv := buildBeadPiArgv(beadPiOptions{theme: "light"}, "scope", "")
	if v, ok := flagValue(argv, "--use-theme"); !ok || v != "light" {
		t.Errorf("--use-theme = %q, want light: %q", v, argv)
	}
}

func TestBuildBeadPiDryRunArgv(t *testing.T) {
	argv := buildBeadPiDryRunArgv(beadPiOptions{narrow: true}, "scope", "plan this")
	if !containsFlag(argv, "-p", "--no-skills", "--no-context-files", "--tools") {
		t.Errorf("narrow dry-run argv wrong: %q", argv)
	}
}

func TestBeadPiScopeNarrowIsSelfSufficient(t *testing.T) {
	stores := []store{{Name: "inbox", About: "capture"}}
	scope := beadPiScope(stores, true)
	for _, want := range []string{
		"--body-file", "0-4", "bug | feature | task | epic | chore | decision",
		"DUPLICATE CHECK", "thin", "CREATED:",
	} {
		if !strings.Contains(scope, want) {
			t.Errorf("narrow scope missing %q", want)
		}
	}
}

func TestBeadPiScopeFullListsStores(t *testing.T) {
	stores := []store{{Name: "inbox", About: "capture"}}
	scope := beadPiScope(stores, false)
	if !strings.Contains(scope, "inbox") {
		t.Errorf("full scope missing store list: %q", scope)
	}
}
