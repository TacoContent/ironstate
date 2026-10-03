//go:build unix && !linux

package cli

import "golang.org/x/sys/unix"

func dupOnto(oldfd, newfd int) error { return unix.Dup2(oldfd, newfd) }
