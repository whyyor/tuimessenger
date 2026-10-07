package tui

import (
	"strings"
	"testing"
)

// mauviewPaste simulates mauview's paste assembly: printable runes are kept,
// CR becomes a newline, and everything else (LF, tab) is dropped.
func mauviewPaste(sent string) string {
	var b strings.Builder
	for _, r := range sent {
		switch {
		case r == '\r':
			b.WriteByte('\n')
		case r >= ' ' && r != 0x7f:
			b.WriteRune(r)
		}
	}
	return b.String()
}

const jobList = "📋 _Shalini – Email-CV Jobs (posted ≤7 days)_\n" +
	"_5 Oct 2026 · Priority: PM > PO > BA > PjM_\n" +
	"━━━━━━━━━━━━━━━\n" +
	"🟢 *PRODUCT MANAGER*\n" +
	"1. AI Product Manager – Holimore\n" +
	"\t📍 Remote (India) · 🎯 3–5 yrs\n" +
	"- item one\n" +
	"- item two"

func TestRestorePasteWhitespaceRestoresLinesAndTabs(t *testing.T) {
	received := mauviewPaste(jobList) // Ghostty sends LF, so lines run together
	if strings.Contains(received, "\n") {
		t.Fatal("simulation should have dropped the line breaks")
	}
	got, ok := restorePasteWhitespace(received, jobList)
	if !ok || got != jobList {
		t.Fatalf("restore failed (ok=%v):\n%q", ok, got)
	}
}

func TestRestorePasteWhitespaceCRLF(t *testing.T) {
	// A clipboard with Windows line endings: Ghostty's CR survives mauview as
	// a newline, so what arrived is already right and nothing is replaced.
	clip := "line one\r\nline two"
	if _, ok := restorePasteWhitespace(mauviewPaste(clip), clip); ok {
		t.Error("CRLF paste already arrived intact; expected no replacement")
	}
	// LF-only clipboard pasted as CR+LF pairs would also normalize cleanly.
	if got, ok := restorePasteWhitespace("line oneline two", "line one\r\nline two"); !ok || got != "line one\nline two" {
		t.Errorf("CRLF clipboard: got %q ok=%v", got, ok)
	}
}

func TestRestorePasteWhitespaceIgnoresUnrelatedClipboard(t *testing.T) {
	// Text dropped onto the terminal doesn't come from the clipboard.
	if _, ok := restorePasteWhitespace("/Users/me/Downloads/file.pdf", "something\nelse"); ok {
		t.Error("used an unrelated clipboard")
	}
	// Single-line paste: nothing to restore.
	if _, ok := restorePasteWhitespace("hello", "hello"); ok {
		t.Error("replaced an identical single-line paste")
	}
}
