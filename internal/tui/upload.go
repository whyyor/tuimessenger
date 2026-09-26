package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// UploadFile uploads the file at path and sends it to the room. The backend
// handles thumbnailing and, for encrypted rooms, file encryption.
func (view *RoomView) UploadFile(path string) {
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
	})
	if err != nil {
		view.AddServiceMessage("Failed to send %s: %v", name, err)
	}
	view.parent.parent.Render()
}

// UploadClipboard extracts an image from the macOS clipboard via pngpaste
// and sends it.
func (view *RoomView) UploadClipboard() {
	defer debug.Recover()
	pngpaste, err := exec.LookPath("pngpaste")
	if err != nil {
		view.AddServiceMessage("pngpaste not found; cannot read images from the clipboard")
		view.parent.parent.Render()
		return
	}
	tmp := filepath.Join(os.TempDir(), "gomuks-clipboard.png")
	defer os.Remove(tmp)
	if out, err := exec.Command(pngpaste, tmp).CombinedOutput(); err != nil {
		view.AddServiceMessage("No image in clipboard: %s", strings.TrimSpace(string(out)))
		view.parent.parent.Render()
		return
	}
	view.UploadFile(tmp)
}
