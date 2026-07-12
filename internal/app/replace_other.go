//go:build !windows

package app

import (
	"errors"
	"os"
)

func atomicReplace(source, destination string, force bool) error {
	if !force {
		if _, err := os.Stat(destination); err == nil {
			return os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(source, destination)
}
