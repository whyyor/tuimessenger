package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
)

// toolUses is what each optional program is for.
var toolUses = map[string]string{
	"ffmpeg": "GIF and video previews",
	"mpv":    "playing GIFs and videos in the terminal",
}

// shownToolHints are the tools whose hint was shown this session.
var shownToolHints sync.Map

// hintMissingTool says once per session how to get a program media needs.
func hintMissingTool(tool string) {
	if _, shown := shownToolHints.LoadOrStore(tool, true); shown {
		return
	}
	toasts.Show(toastInfo, toolHint(tool, runtime.GOOS, havePath))
}

func havePath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// toolHint is the hint for a missing program, with the install command for this system.
func toolHint(tool, goos string, have func(string) bool) string {
	return fmt.Sprintf("Install %s for %s: %s", tool, toolUses[tool], installCommand(tool, goos, have))
}

func installCommand(tool, goos string, have func(string) bool) string {
	switch {
	case goos == "darwin":
		return "brew install " + tool
	case have("apt-get"):
		return "sudo apt install " + tool
	case have("dnf"):
		return "sudo dnf install " + tool
	case have("pacman"):
		return "sudo pacman -S " + tool
	case have("brew"):
		return "brew install " + tool
	}
	return "install " + tool + " with your package manager"
}
