package term

import "testing"

func TestEncodePasteToBytes(t *testing.T) {
	t.Run("plain paste", func(t *testing.T) {
		got := EncodePasteToBytes("echo hi\n", false)
		if string(got) != "echo hi\n" {
			t.Fatalf("unexpected plain paste encoding: %q", string(got))
		}
	})

	t.Run("bracketed paste", func(t *testing.T) {
		got := EncodePasteToBytes("echo hi\n", true)
		want := "\x1b[200~echo hi\n\x1b[201~"
		if string(got) != want {
			t.Fatalf("unexpected bracketed paste encoding: %q", string(got))
		}
	})
}

func TestTerminalPaneBracketedPasteDetection(t *testing.T) {
	p := NewTerminalPane(nil)

	p.mu.Lock()
	p.updateModesLocked([]byte("abc\x1b[?2004h"))
	p.mu.Unlock()
	if !p.BracketedPasteEnabled() {
		t.Fatalf("expected bracketed paste to be enabled")
	}

	p.mu.Lock()
	p.updateModesLocked([]byte("xyz\x1b[?2004l"))
	p.mu.Unlock()
	if p.BracketedPasteEnabled() {
		t.Fatalf("expected bracketed paste to be disabled")
	}
}

func TestTerminalPaneBracketedPasteDetectionAcrossChunks(t *testing.T) {
	p := NewTerminalPane(nil)

	p.mu.Lock()
	p.updateModesLocked([]byte("\x1b[?20"))
	p.updateModesLocked([]byte("04h"))
	p.mu.Unlock()
	if !p.BracketedPasteEnabled() {
		t.Fatalf("expected bracketed paste enable sequence across chunks to be detected")
	}
}
