//go:build linux

package cli

import "golang.org/x/sys/unix"

func dupOnto(oldfd, newfd int) error { return unix.Dup3(oldfd, newfd, 0) }
