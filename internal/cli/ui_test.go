package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/matryer/is"
)

// stripAnsi removes the SGR escape sequences the diagnostic helpers
// inject around their labels. Walks each escape from "\x1b[" through
// the next 'm', skipping only digits and ';' in between so a stray
// 'm' in the message body doesn't truncate an escape early.
func stripAnsi(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++ // skip the 'm'
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func newTestUI(buf *bytes.Buffer) *ui {
	return newUI(buf, false, false)
}

// Diagnostics with multi-line messages should align continuation
// lines under the message body, not at column 0. Otherwise the
// trailing lines look unrelated to the warning label.
func TestUIWarning_MultiLineAligns(t *testing.T) {
	assert := is.New(t)

	var buf bytes.Buffer
	ui := newTestUI(&buf)
	ui.Warning("first line\nsecond line\nthird line")

	out := stripAnsi(buf.String())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	assert.Equal(len(lines), 3)
	assert.Equal(lines[0], "warning: first line")
	// Continuation lines indent by len("warning: ") = 9 spaces.
	assert.Equal(lines[1], "         second line")
	assert.Equal(lines[2], "         third line")
}

func TestUIError_MultiLineAligns(t *testing.T) {
	assert := is.New(t)

	var buf bytes.Buffer
	ui := newTestUI(&buf)
	ui.Error("oops\ndetail")

	out := stripAnsi(buf.String())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	assert.Equal(lines[0], "error: oops")
	// "error: " = 7 chars.
	assert.Equal(lines[1], "       detail")
}

func TestUIHint_MultiLineAligns(t *testing.T) {
	assert := is.New(t)

	var buf bytes.Buffer
	ui := newTestUI(&buf)
	ui.Hint("try this\nor that")

	out := stripAnsi(buf.String())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	assert.Equal(lines[0], "hint: try this")
	// "hint: " = 6 chars.
	assert.Equal(lines[1], "      or that")
}

// Single-line messages keep the existing one-line shape — no
// continuation logic engages.
func TestUIWarning_SingleLineUnchanged(t *testing.T) {
	assert := is.New(t)

	var buf bytes.Buffer
	ui := newTestUI(&buf)
	ui.Warning("just one line")

	out := stripAnsi(buf.String())
	assert.Equal(strings.TrimRight(out, "\n"), "warning: just one line")
}
