package runtime

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

var driveTargets sync.Map

func asciiPath(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// A per-logon DOS device handles volumes with 8.3 names disabled. No files move.
func driveAlias(path string) string {
	root := filepath.Dir(path)
	target := `\??\` + root
	if strings.HasPrefix(root, `\\`) {
		target = `\??\UNC\` + strings.TrimPrefix(root, `\\`)
	}
	logical, e := windows.GetLogicalDrives()
	if e != nil {
		return path
	}
	for letter := 'Z'; letter >= 'G'; letter-- {
		name := string(letter) + ":"
		p, _ := windows.UTF16PtrFromString(name)
		buf := make([]uint16, 32768)
		if _, e := windows.QueryDosDevice(p, &buf[0], uint32(len(buf))); e == nil {
			if windows.UTF16ToString(buf) == target {
				driveTargets.Store(name, target)
				return name + `\` + filepath.Base(path)
			}
			continue
		}
		if logical&(1<<uint(letter-'A')) != 0 {
			continue
		}
		dest, _ := windows.UTF16PtrFromString(target)
		if e := windows.DefineDosDevice(1|8, p, dest); e == nil {
			driveTargets.Store(name, target)
			return name + `\` + filepath.Base(path)
		}
	}
	return path
}
func ReleaseToolPath(path string) {
	if len(path) < 2 {
		return
	}
	name := path[:2]
	target, ok := driveTargets.LoadAndDelete(name)
	if !ok {
		return
	}
	p, _ := windows.UTF16PtrFromString(name)
	dest, _ := windows.UTF16PtrFromString(target.(string))
	windows.DefineDosDevice(1|2|4|8, p, dest)
}

// PostgreSQL's Windows C tools use the active ANSI code page in filesystem-derived
// bootstrap SQL. Use an existing 8.3 alias without changing the user-visible path.
func ToolPath(path string) string {
	if os.Getenv("OPENFOOD_TEST_FORCE_DRIVE_ALIAS") == "1" {
		return driveAlias(path)
	}
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return path
	}
	buf := make([]uint16, 32768)
	n, e := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if e != nil || n == 0 || n >= uint32(len(buf)) {
		if !asciiPath(path) {
			return driveAlias(path)
		}
		return path
	}
	short := windows.UTF16ToString(buf[:n])
	if !asciiPath(short) {
		return driveAlias(path)
	}
	return short
}
func EnsureToolDir(path string) (string, error) {
	if e := os.MkdirAll(path, 0700); e != nil {
		return "", e
	}
	return ToolPath(path), nil
}

func Hide(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func Protect(path string) error {
	// Protected directory DACL: only this Windows user and SYSTEM inherit access.
	token := windows.GetCurrentProcessToken()
	user, e := token.GetTokenUser()
	if e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)")
	if e != nil {
		return e
	}
	dacl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
