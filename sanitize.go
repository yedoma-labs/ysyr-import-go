package ysyrimport

// maxMessageLength is the maximum length, in runes, of any message or column name in a RowError.
const maxMessageLength = 100

// isUnsafeRune reports whether r can alter the layout of a log line or a report: control
// characters, line and paragraph separators, and bidirectional controls.
func isUnsafeRune(r rune) bool {
	return r <= 0x1f ||
		(r >= 0x7f && r <= 0x9f) ||
		r == 0x2028 || r == 0x2029 ||
		(r >= 0x202a && r <= 0x202e) ||
		(r >= 0x2066 && r <= 0x2069)
}

// sanitizeText replaces unsafe runes with a space and truncates the text to at most 100 runes.
// A truncated text ends with an ellipsis, so the result is still 100 runes long.
func sanitizeText(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		if isUnsafeRune(r) {
			runes[i] = ' '
		}
	}
	if len(runes) <= maxMessageLength {
		return string(runes)
	}
	return string(runes[:maxMessageLength-1]) + "…"
}

// errorText returns the sanitized message of an error returned by a parser or a validator.
func errorText(err error) string {
	text := sanitizeText(err.Error())
	if text == "" {
		return "invalid value"
	}
	return text
}
