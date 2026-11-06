//go:build !linux

package storage

import "errors"

func FreeBytes(string) (uint64, error) {
	return 0, errors.New("free-space check is available on Linux only")
}
