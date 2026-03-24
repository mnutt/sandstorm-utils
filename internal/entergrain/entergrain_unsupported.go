//go:build !linux || !amd64

package entergrain

import "errors"

func enter(pid int, shellPath string, env []string) error {
	return errors.New("enter-grain is only supported on Linux")
}
