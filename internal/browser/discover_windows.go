//go:build windows

package browser

import (
	"errors"
	"path/filepath"
)

// FindExecutable returns the first installed supported browser in priority order.
func FindExecutable(getenv func(string) string, exists func(string) bool) (string, error) {
	type location struct {
		env      string
		relative string
	}

	candidates := []location{
		{"PROGRAMFILES(X86)", `Microsoft\Edge\Application\msedge.exe`},
		{"PROGRAMFILES", `Microsoft\Edge\Application\msedge.exe`},
		{"LOCALAPPDATA", `Microsoft\Edge\Application\msedge.exe`},
		{"PROGRAMFILES(X86)", `Google\Chrome\Application\chrome.exe`},
		{"PROGRAMFILES", `Google\Chrome\Application\chrome.exe`},
		{"LOCALAPPDATA", `Google\Chrome\Application\chrome.exe`},
	}

	for _, candidate := range candidates {
		base := getenv(candidate.env)
		if base == "" {
			continue
		}
		path := filepath.Join(base, candidate.relative)
		if exists(path) {
			return path, nil
		}
	}

	return "", errors.New("supported browser not found")
}
