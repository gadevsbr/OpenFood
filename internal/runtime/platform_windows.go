package runtime

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"syscall"
)

// PostgreSQL's Windows C tools use the active ANSI code page in filesystem-derived
// bootstrap SQL. Use an existing 8.3 alias without changing the user-visible path.
func ToolPath(path string) string {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return path
	}
	buf := make([]uint16, 32768)
	n, e := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if e != nil || n == 0 || n >= uint32(len(buf)) {
		return path
	}
	return windows.UTF16ToString(buf[:n])
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
