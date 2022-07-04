//+build !windows,!plan9

package fileutil

import (
	"golang.org/x/sys/unix"
	"os"
)



func mmap(f *os.File, length int) ([]byte, error) {
	return unix.Mmap(int(f.Fd()), 0, length, unix.PROT_READ, unix.MAP_SHARED)
}

func munmap(b []byte) (err error) {
	return unix.Munmap(b)
}
