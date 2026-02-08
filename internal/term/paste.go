package term

// EncodePasteToBytes returns bytes to send for pasted content. When bracketed
// paste mode is enabled by the shell/editor, the payload is wrapped with
// xterm bracketed-paste markers.
func EncodePasteToBytes(content string, bracketed bool) []byte {
	if content == "" {
		return nil
	}
	if !bracketed {
		return []byte(content)
	}
	out := make([]byte, 0, len(content)+len(bracketedPasteOnSeq)+len(bracketedPasteOffSeq))
	out = append(out, []byte("\x1b[200~")...)
	out = append(out, content...)
	out = append(out, []byte("\x1b[201~")...)
	return out
}
