package runtime

import (
	"os"
	"os/exec"
)

func ToolPath(path string) string { return path }
func ReleaseToolPath(path string) {}
func EnsureToolDir(path string) (string, error) {
	if e := os.MkdirAll(path, 0700); e != nil {
		return "", e
	}
	return path, nil
}

func Hide(cmd *exec.Cmd)        {}
func Protect(path string) error { return nil }
