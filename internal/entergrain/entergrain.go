package entergrain

import (
	"fmt"
	"os"
	"strconv"
)

type Options struct {
	ShellPath string
}

func Run(pidArg string, options Options) error {
	pid, err := strconv.Atoi(pidArg)
	if err != nil || pid <= 0 {
		return fmt.Errorf("invalid pid %q", pidArg)
	}

	shellPath := options.ShellPath
	if shellPath == "" {
		shellPath = "/bin/bash"
	}

	env, err := readProcessEnv(pid)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "Attaching to process ID %d...\n", pid)
	return enter(pid, shellPath, env)
}

func parseEnviron(data []byte) []string {
	if len(data) == 0 {
		return nil
	}

	var env []string
	start := 0
	for i, b := range data {
		if b != 0 {
			continue
		}
		if i > start {
			env = append(env, string(data[start:i]))
		}
		start = i + 1
	}
	if start < len(data) {
		env = append(env, string(data[start:]))
	}
	return env
}

func readProcessEnv(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil, fmt.Errorf("read /proc/%d/environ: %w", pid, err)
	}
	return parseEnviron(data), nil
}
