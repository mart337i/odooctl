//go:build !windows

package selfupdate

import "os"

func replaceExecutable(target, replacement string) error {
	return os.Rename(replacement, target)
}

func replaceWindows(target, replacement string) error {
	return replaceExecutable(target, replacement)
}
