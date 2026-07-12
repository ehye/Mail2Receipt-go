//go:build !windows

package app

import "os"

func atomicReplace(source, destination string, force bool) error {
	if !force {
		if err := os.Link(source, destination); err != nil {
			return err
		}
		_ = os.Remove(source)
		return nil
	}
	return os.Rename(source, destination)
}
