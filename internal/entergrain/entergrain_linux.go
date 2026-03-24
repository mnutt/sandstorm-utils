//go:build linux && amd64

package entergrain

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

type namespaceSpec struct {
	name string
	flag uintptr
	fd   int
}

func enter(pid int, shellPath string, env []string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	namespaces := []namespaceSpec{
		{name: "user", flag: syscall.CLONE_NEWUSER},
		{name: "ipc", flag: syscall.CLONE_NEWIPC},
		{name: "uts", flag: syscall.CLONE_NEWUTS},
		{name: "net", flag: syscall.CLONE_NEWNET},
		{name: "pid", flag: syscall.CLONE_NEWPID},
		{name: "mnt", flag: syscall.CLONE_NEWNS},
	}

	for i := range namespaces {
		fd, err := syscall.Open(fmt.Sprintf("/proc/%d/ns/%s", pid, namespaces[i].name), syscall.O_RDONLY, 0)
		if err != nil {
			return fmt.Errorf("open /proc/%d/ns/%s: %w", pid, namespaces[i].name, err)
		}
		namespaces[i].fd = fd
	}
	cwdFD, err := syscall.Open(fmt.Sprintf("/proc/%d/cwd", pid), syscall.O_RDONLY, 0)
	if err != nil {
		closeNamespaces(namespaces)
		return fmt.Errorf("open /proc/%d/cwd: %w", pid, err)
	}
	defer syscall.Close(cwdFD)

	if err := syscall.Setgroups([]int{}); err != nil {
		closeNamespaces(namespaces)
		return fmt.Errorf("clear supplementary groups: %w", err)
	}

	for _, ns := range namespaces {
		if err := setns(ns.fd, ns.flag); err != nil {
			closeNamespaces(namespaces)
			return fmt.Errorf("setns %s: %w", ns.name, err)
		}
		if err := syscall.Close(ns.fd); err != nil {
			return fmt.Errorf("close %s namespace fd: %w", ns.name, err)
		}
	}

	if err := syscall.Fchdir(cwdFD); err != nil {
		return fmt.Errorf("fchdir target cwd: %w", err)
	}

	stat, err := os.Stat("/var")
	if err != nil {
		return fmt.Errorf("stat /var: %w", err)
	}
	sys, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("stat /var: unexpected stat payload %T", stat.Sys())
	}

	attr := &syscall.ProcAttr{
		Env:   env,
		Files: []uintptr{0, 1, 2},
		Sys: &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: sys.Uid,
				Gid: sys.Gid,
			},
		},
	}

	childPID, err := syscall.ForkExec(shellPath, []string{shellPath}, attr)
	if err != nil {
		return fmt.Errorf("exec %s: %w", shellPath, err)
	}

	var waitStatus syscall.WaitStatus
	var rusage syscall.Rusage
	if _, err := syscall.Wait4(childPID, &waitStatus, 0, &rusage); err != nil {
		return fmt.Errorf("wait for child: %w", err)
	}

	if waitStatus.Exited() && waitStatus.ExitStatus() == 0 {
		return nil
	}
	if waitStatus.Exited() {
		return fmt.Errorf("%s exited with status %d", shellPath, waitStatus.ExitStatus())
	}
	if waitStatus.Signaled() {
		return fmt.Errorf("%s terminated by signal %s", shellPath, waitStatus.Signal())
	}
	return fmt.Errorf("%s exited unexpectedly", shellPath)
}

func closeNamespaces(namespaces []namespaceSpec) {
	for _, ns := range namespaces {
		if ns.fd > 0 {
			_ = syscall.Close(ns.fd)
		}
	}
}

func setns(fd int, nstype uintptr) error {
	const sysSetns = 308
	_, _, errno := syscall.Syscall(sysSetns, uintptr(fd), nstype, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
