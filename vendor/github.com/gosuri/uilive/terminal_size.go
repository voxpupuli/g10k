// +build !windows

package uilive

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

type windowSize struct {
	rows    uint16
	cols    uint16
}

func getTermSize() (int, int) {
	var sz windowSize
	var f *os.File
	var err error
	if runtime.GOOS == "openbsd" {
		f, err = os.OpenFile("/dev/tty", os.O_RDWR, 0)
	} else {
		f, err = os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	}
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&sz)))
	return int(sz.cols), int(sz.rows)
}
