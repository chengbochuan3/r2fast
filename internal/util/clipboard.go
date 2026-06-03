package util

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// CopyClipboard copies text to the system clipboard, best-effort.
func CopyClipboard(s string) error {
	switch runtime.GOOS {
	case "darwin":
		return pipe("pbcopy", nil, s)
	case "windows":
		return pipe("clip", nil, s)
	case "linux":
		if p, err := exec.LookPath("wl-copy"); err == nil {
			return pipe(p, nil, s)
		}
		if p, err := exec.LookPath("xclip"); err == nil {
			return pipe(p, []string{"-selection", "clipboard"}, s)
		}
		if p, err := exec.LookPath("xsel"); err == nil {
			return pipe(p, []string{"--clipboard", "--input"}, s)
		}
		return errors.New("no clipboard tool found (install wl-clipboard or xclip)")
	}
	return errors.New("clipboard not supported on this platform")
}

func pipe(name string, args []string, input string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	return cmd.Run()
}
