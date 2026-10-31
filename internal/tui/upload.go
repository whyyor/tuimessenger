package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc"
	"go.mau.fi/gomuks/tui/debug"
)

// cleanDroppedPath normalizes a path as typed by terminal drag and drop:
// shell-escaped spaces, surrounding quotes, and ~.
func cleanDroppedPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, `"'`)
	path = strings.ReplaceAll(path, `\ `, " ")
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return path
}

// isFile reports a path to an existing regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// splitShellWords splits "a\ b.pdf 'c d.png' e" the way a terminal drop
// escapes paths: backslash escapes and quotes, whitespace between.
func splitShellWords(text string) []string {
	var words []string
	var cur strings.Builder
	var quote rune
	inWord, escaped := false, false
	for _, r := range text {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, inWord = true, true
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '\'' || r == '"'):
			quote, inWord = r, true
		case quote == 0 && (r == ' ' || r == '\t' || r == '\n'):
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}

// droppedFiles returns the files in pasted or dropped text, or nil unless
// every word is an existing file.
func droppedFiles(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" || strings.Count(text, "\n") > 20 {
		return nil
	}
	// A single unescaped path with spaces, as typed after /upload.
	if path := cleanDroppedPath(text); isFile(path) {
		return []string{path}
	}
	words := splitShellWords(text)
	if len(words) == 0 {
		return nil
	}
	paths := make([]string, len(words))
	for i, word := range words {
		paths[i] = cleanDroppedPath(word)
		if !isFile(paths[i]) {
			return nil
		}
	}
	return paths
}

// clipboardFilesScript lists file URLs on the pasteboard (Finder's ⌘C).
const clipboardFilesScript = `ObjC.import('AppKit');
var urls = $.NSPasteboard.generalPasteboard.readObjectsForClassesOptions($([$.NSURL]), $({NSPasteboardURLReadingFileURLsOnlyKey: true}));
var out = [];
if (urls) for (var i = 0; i < urls.count; i++) out.push(urls.objectAtIndex(i).path.js);
out.join('\n');`

func clipboardFiles() []string {
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", clipboardFilesScript).Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if isFile(line) {
			paths = append(paths, line)
		}
	}
	return paths
}

// pickFilesScript opens the native file dialog over the terminal.
const pickFilesScript = `tell application (path to frontmost application as text)
	set picked to choose file with prompt "Send to chat" with multiple selections allowed
end tell
set out to ""
repeat with f in picked
	set out to out & POSIX path of f & linefeed
end repeat
return out`

// pickFiles returns the chosen files; nil with no error means cancelled.
func pickFiles() ([]string, error) {
	out, err := exec.Command("osascript", "-e", pickFilesScript).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && strings.Contains(string(exitErr.Stderr), "-128") {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// finderPicker is the Finder dialog; swapped out in tests.
var finderPicker = pickFiles

// suspendTerminal hands the terminal to fn via the UI loop; swapped in tests.
var suspendTerminal = func(view *RoomView, fn func()) {
	view.parent.parent.app.Suspend(fn)
}

// pickWithYazi runs yazi full-screen as a chooser: Enter on a file (Space
// marks several) picks it, q cancels. It starts in ~/Downloads.
func (view *RoomView) pickWithYazi(yazi string) ([]string, error) {
	// Not "gomuks-…": the media cache cleaner deletes those from the temp dir.
	f, err := os.CreateTemp("", "yazi-chooser-*")
	if err != nil {
		return nil, err
	}
	chooser := f.Name()
	_ = f.Close()
	defer os.Remove(chooser)
	start, _ := os.UserHomeDir()
	if dl := filepath.Join(start, "Downloads"); dirExists(dl) {
		start = dl
	}
	// Suspend only queues the function for the UI loop, so wait for yazi to
	// exit before reading its choice. Callers run off the UI goroutine.
	var runErr error
	done := make(chan struct{})
	suspendTerminal(view, func() {
		defer close(done)
		print("\033[2J\033[0;0H")
		cmd := exec.Command(yazi, "--chooser-file="+chooser, start)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		runErr = cmd.Run()
		print("\033[2J\033[0;0H")
	})
	<-done
	RequestRepaint()
	if runErr != nil {
		return nil, runErr
	}
	return readChooserFile(chooser)
}

// readChooserFile reads the paths yazi wrote, one per line; none means cancelled.
func readChooserFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// SendFiles is /upload: no argument opens the file picker to attach files;
// with paths (one, or several as a terminal drop escapes them) it sends now.
func (view *RoomView) SendFiles(arg string) {
	defer debug.Recover()
	var paths []string
	if strings.TrimSpace(arg) == "" {
		view.AttachPicked()
		return
	} else if paths = droppedFiles(arg); paths == nil {
		view.AddServiceMessage("No such file: %s", cleanDroppedPath(arg))
		view.parent.parent.Render()
		return
	}
	for _, path := range paths {
		view.UploadFile(path)
	}
}

// UploadFile uploads the file at path and sends it to the room. The backend
// picks image, video, audio or plain file from the content, thumbnails and,
// for encrypted rooms, encrypts it.
func (view *RoomView) UploadFile(path string) {
	view.uploadAndSend(path, "", nil)
}

// uploadAndSend uploads one file and sends it, with an optional caption
// (markdown, like messages) and reply.
func (view *RoomView) uploadAndSend(path, caption string, relatesTo *event.RelatesTo) {
	defer debug.Recover()
	path = cleanDroppedPath(path)
	file, err := os.Open(path)
	if err != nil {
		view.AddServiceMessage("Failed to open %s: %v", path, err)
		view.parent.parent.Render()
		return
	}
	defer file.Close()

	name := filepath.Base(path)
	view.AddServiceMessage("Uploading %s…", name)
	view.parent.parent.Render()

	meta := view.Room.Meta.Current()
	encrypt := meta != nil && meta.EncryptionEvent != nil
	rpcClient := view.parent.matrix.GomuksAPI.(*rpc.GomuksRPC)
	content, err := rpcClient.UploadMedia(context.TODO(), name, encrypt, file)
	if err != nil {
		view.AddServiceMessage("Failed to upload %s: %v", name, err)
		view.parent.parent.Render()
		return
	}
	err = view.parent.matrix.SendMessage(context.TODO(), &jsoncmd.SendMessageParams{
		RoomID:      view.Room.ID,
		BaseContent: content,
		Text:        caption,
		RelatesTo:   relatesTo,
	})
	if err != nil {
		view.AddServiceMessage("Failed to send %s: %v", name, err)
	}
	view.parent.parent.Render()
}
