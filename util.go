package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// ANSI colors for the picker/list. Enabled only on a tty, honoring NO_COLOR.
// colorOn gates styled output: tty without NO_COLOR/TERM=dumb.
// See theme.go — styles come from the active palette.
var colorOn = false

func padRight(s string, w int) string {
	for len(s) < w {
		s += " "
	}
	return s
}

func shorten(s string, n int) string {
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

func looksLikeFlag(s string) bool {
	return strings.HasPrefix(s, "-")
}

func isTTY(f *os.File) bool {
	// ioctl, not mode bits: /dev/null is a char device but not a tty,
	// and bash's [ -t 0 ] does the real isatty check. Matches TIOCGWINSZ
	// on darwin and linux.
	type winsize struct {
		rows, cols, x, y uint16
	}
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(),
		syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	return errno == 0
}

func execStore(storeName string, args []string) int {
	cmd := exec.Command(storeName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bead: running %s: %v\n", storeName, err)
		return 1
	}
	return 0
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func readAll(f *os.File) ([]byte, error) {
	return io.ReadAll(f)
}

func writeTemp(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}
