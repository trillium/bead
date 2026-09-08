package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const sentinel = ">>>>> DESCRIPTION BELOW — keep this line; everything under it is the body >>>>>"

// runEditorForm is the legacy editor flow, kept for when stdout is not a
// tty and the fullscreen TUI cannot run.
func runEditorForm(storeName, prefillTitle string, dryRun bool) int {
	form, err := os.CreateTemp("", "bead-form-*.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead: %v\n", err)
		return 1
	}
	formPath := form.Name()
	fmt.Fprintf(form, "Title: %s\nType: task\nPriority: 2\nLabels:\n", prefillTitle)
	fmt.Fprintf(form, "# Type: bug|feature|task|epic|chore|decision    Priority: 0-4 (0=highest)\n")
	fmt.Fprintf(form, "# Lines starting with # are ignored. Keep the Title short (one line).\n")
	fmt.Fprintf(form, "%s\n\n", sentinel)
	form.Close()

	editor := resolveEditor()
	if editor == "" {
		fmt.Fprintf(os.Stderr, "bead: no editor found; set $EDITOR or $BEAD_EDITOR\n")
		return 1
	}
	// Run through sh so values like `code -g --wait` keep working.
	editCmd := exec.Command("sh", "-c", editor+` "$1"`, "bead-sh", formPath)
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr
	if err := editCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bead: editor exited non-zero; draft kept at %s\n", formPath)
		return 1
	}

	raw, err := os.ReadFile(formPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bead: %v\n", err)
		return 1
	}
	header, body := splitForm(string(raw))
	title := field(header, "Title")
	typ := field(header, "Type")
	if typ == "" {
		typ = "task"
	}
	prio := field(header, "Priority")
	if prio == "" {
		prio = "2"
	}
	labels := field(header, "Labels")

	body = dropLeadingBlanks(body)
	if len(bytesTrimSpace([]byte(body))) == 0 {
		body = ""
	}
	if title == "" {
		fmt.Fprintf(os.Stderr, "bead: empty Title — aborted. Your draft is kept at: %s\n", formPath)
		return 1
	}

	args := []string{"create", title, "-t", typ, "-p", prio}
	if labels != "" {
		args = append(args, "-l", labels)
	}
	var bodyFile string
	if body != "" {
		bodyFile, err = writeTemp("bead-body.*", []byte(body+"\n"))
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
	if bodyFile != "" {
		os.Remove(bodyFile)
	}
	if dryRun {
		fmt.Fprintf(os.Stderr, "bead: dry run — nothing created; draft kept at %s\n", formPath)
		return rc
	}
	if rc == 0 {
		os.Remove(formPath)
	} else {
		fmt.Fprintf(os.Stderr, "bead: create failed (rc=%d); draft kept at %s\n", rc, formPath)
	}
	return rc
}

func resolveEditor() string {
	for _, k := range []string{"BEAD_EDITOR", "VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	for _, c := range []string{"nano", "vim", "vi"} {
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return ""
}

func splitForm(raw string) (header, body string) {
	lines := strings.Split(raw, "\n")
	sentinelIdx := -1
	for i, l := range lines {
		if strings.TrimRight(l, "\r") == sentinel {
			sentinelIdx = i
			break
		}
	}
	if sentinelIdx < 0 {
		return raw, ""
	}
	header = strings.Join(lines[:sentinelIdx], "\n")
	body = strings.Join(lines[sentinelIdx+1:], "\n")
	return header, body
}

// field extracts the first `^Name:` value from the form header, ignoring
// `#` comment lines (they never match `^Name:` anyway).
func field(header, name string) string {
	prefix := name + ":"
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func dropLeadingBlanks(s string) string {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	return strings.Join(lines[i:], "\n")
}
