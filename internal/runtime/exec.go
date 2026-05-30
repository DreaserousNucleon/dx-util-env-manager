package runtime

import (
	"io"
	"os"
	"os/exec"
)

func Exec(name string, args []string, env []string, out io.Writer, errOut io.Writer) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Stdin = os.Stdin
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}

	return 0, nil
}
