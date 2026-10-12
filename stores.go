package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// loadStores returns the authoritative store list. On a brain remote client
// (~/.config/brain/remote.yaml exists) the local stores.yaml is a stale copy,
// so ask the host through `brain stores list --json`; on the host, or if the
// remote cannot be reached, read the local registry.
func loadStores() ([]store, error) {
	return chooseStores(isRemoteClient(), fetchRemoteStores, loadLocalStores)
}

// chooseStores picks the remote list on a remote client when it yields stores,
// else the local registry.
func chooseStores(remote bool, fetch, local func() ([]store, error)) ([]store, error) {
	if remote {
		if stores, err := fetch(); err == nil && len(stores) > 0 {
			return stores, nil
		}
	}
	return local()
}

func isRemoteClient() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".config", "brain", "remote.yaml"))
	return err == nil
}

// fetchRemoteStores asks the brain host for its registry; brain forwards the
// command over ssh when this machine is a remote client.
func fetchRemoteStores() ([]store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "brain", "stores", "list", "--json").Output()
	if err != nil {
		return nil, err
	}
	return parseStoresJSON(out)
}

// parseStoresJSON decodes `brain stores list --json`. The host may print
// notices before the JSON array.
func parseStoresJSON(data []byte) ([]store, error) {
	start := bytes.IndexByte(data, '[')
	if start < 0 {
		return nil, fmt.Errorf("no JSON array in stores list output")
	}
	var rows []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		About string `json:"about"`
	}
	if err := json.NewDecoder(bytes.NewReader(data[start:])).Decode(&rows); err != nil {
		return nil, fmt.Errorf("parsing stores list: %w", err)
	}
	var out []store
	for _, r := range rows {
		if r.Name != "" {
			out = append(out, store{Name: r.Name, Path: r.Path, About: r.About})
		}
	}
	return out, nil
}

// loadLocalStores parses the small known shape of stores.yaml without a yaml
// dependency:
//
//	stores:
//	    <name>:
//	        path: ...
//	        about: ...
func loadLocalStores() ([]store, error) {
	path := registryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading store registry %s: %w", path, err)
	}
	var out []store
	var cur *store
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if strings.HasPrefix(trimmed, "    ") && !strings.HasPrefix(trimmed, "        ") {
			// `    name:` — a store key (4-space indent, no deeper indent).
			rest := strings.TrimSpace(trimmed)
			if strings.HasSuffix(rest, ":") {
				name := strings.TrimSuffix(rest, ":")
				if name != "" && name != "stores" && !strings.Contains(name, " ") {
					out = append(out, store{Name: name})
					cur = &out[len(out)-1]
				} else {
					cur = nil
				}
				continue
			}
			cur = nil
			continue
		}
		if cur != nil {
			inner := strings.TrimSpace(trimmed)
			if strings.HasPrefix(inner, "about:") {
				cur.About = unquote(strings.TrimSpace(strings.TrimPrefix(inner, "about:")))
			} else if strings.HasPrefix(inner, "path:") {
				cur.Path = unquote(strings.TrimSpace(strings.TrimPrefix(inner, "path:")))
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no stores found in %s", path)
	}
	return out, nil
}

func registryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "pai", "stores.yaml")
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			inner := s[1 : len(s)-1]
			// YAML single-quote escape: '' → '
			if s[0] == '\'' {
				inner = strings.ReplaceAll(inner, "''", "'")
			}
			return inner
		}
	}
	return s
}

func printStores(stores []store) {
	th := activeTheme()
	maxName := 0
	for _, s := range stores {
		if len(s.Name) > maxName {
			maxName = len(s.Name)
		}
	}
	for _, s := range stores {
		name := th.Name.Render(padRight(s.Name, maxName))
		about := s.About
		if len(about) > 90 {
			about = about[:87] + "..."
		}
		if about != "" {
			fmt.Printf("  %s  %s\n", name, th.Dim.Render(about))
		} else {
			fmt.Printf("  %s\n", name)
		}
	}
}

// pickStore runs the fullscreen TUI picker on a real terminal, else the
// plain line prompt (piped stdout, dumb terminals).
func pickStore(stores []store) (store, bool) {
	if isTTY(os.Stdin) && isTTY(os.Stdout) {
		return runPickerTUI(stores)
	}
	return pickStoreLine(stores)
}

// pickStoreLine lists every store and prompts for one by name
// (case-insensitive); q quits. Fallback for non-terminal output.
func pickStoreLine(stores []store) (store, bool) {
	byName := map[string]store{}
	for _, s := range stores {
		byName[strings.ToLower(s.Name)] = s
	}

	fmt.Println("Select a bead store:")
	printStores(stores)

	in := openPromptInput()
	defer func() {
		if f, ok := in.(*os.File); ok && f != os.Stdin {
			f.Close()
		}
	}()
	reader := bufio.NewReader(in)
	for {
		fmt.Printf("\nstore [name, or q to quit]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return store{}, false
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		if choice == "" {
			continue
		}
		if choice == "q" || choice == "quit" || choice == "exit" {
			return store{}, false
		}
		if s, ok := byName[choice]; ok {
			return s, true
		}
		fmt.Fprintf(os.Stderr, "  unknown store %q — try a store name from the list above.\n",
			strings.TrimSpace(line))
	}
}

// openPromptInput returns /dev/tty for the picker when available so the
// prompt works even if stdin is redirected; falls back to stdin.
func openPromptInput() interface {
	Read([]byte) (int, error)
} {
	if f, err := os.Open("/dev/tty"); err == nil {
		return f
	}
	return os.Stdin
}
