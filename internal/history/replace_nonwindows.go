//go:build !windows

package history

import "os"

func replaceFile(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
