//go:build !windows

package browser

import "errors"

// FindExecutable reports that browser discovery is only available on Windows.
func FindExecutable(getenv func(string) string, exists func(string) bool) (string, error) {
	return "", errors.New("browser discovery is unsupported on this platform")
}
