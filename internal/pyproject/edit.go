package pyproject

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"pensa.sh/pensa/pkg/pep508"
)

// DepEditOp is the operation requested against a dependency entry.
type DepEditOp int

const (
	// EditAdd appends entry, or replaces an existing entry whose
	// package name matches entryName (case-insensitive PEP 503).
	EditAdd DepEditOp = iota + 1
	// EditRemove deletes the entry whose package name matches
	// entryName. Returns nil if the entry isn't found.
	EditRemove
)

// EditDepArray modifies a TOML string-array at path table.key, preserving
// the file's existing format (indent, quote style, comments). Used for
// PEP 621 [project].dependencies and PEP 735 [dependency-groups].<group>.
//
// Format detection:
//   - Single-line array (`key = ["a", "b"]`) is expanded to multi-line
//     on first edit so subsequent additions stay clean.
//   - Indent and quote style are copied from existing entries; falls
//     back to 4-space + double-quote for empty arrays.
//   - Trailing commas are preserved when present in the source.
//
// Returns the modified bytes. Returns the input unchanged when EditRemove
// targets a name that isn't present.
func EditDepArray(data []byte, table, key string, op DepEditOp, entry, entryName string) ([]byte, error) {
	span, err := findArraySpan(data, table, key)
	if err != nil {
		return nil, err
	}

	if span == nil {
		// Array doesn't exist. Add operation creates one; remove is
		// a no-op.
		if op == EditRemove {
			return data, nil
		}
		return appendArrayKey(data, table, key, entry)
	}

	// Single-line array: expand to multi-line first so insertions
	// don't pile entries onto one runaway line.
	if span.singleLine {
		data, span, err = expandArrayToMultiline(data, span)
		if err != nil {
			return nil, err
		}
	}

	style := detectArrayStyle(data, span)
	target := pep508.NormalizeName(entryName)

	// Walk the existing entries.
	for _, e := range span.entries {
		text := string(data[e.valueStart:e.valueEnd])
		name := depEntryName(text)
		if pep508.NormalizeName(name) != target {
			continue
		}
		switch op {
		case EditAdd:
			// Replace in place: substitute the quoted text only,
			// keeping surrounding whitespace, comma, and any
			// trailing comment intact.
			return spliceBytes(data, e.valueStart, e.valueEnd, []byte(quoteEntry(entry, style.quote))), nil
		case EditRemove:
			return removeLine(data, e.lineStart, e.lineEnd), nil
		}
	}

	// No match.
	if op == EditRemove {
		return data, nil
	}
	return insertNewEntry(data, span, style, entry), nil
}

// arraySpan describes the byte ranges of an array literal in the file.
type arraySpan struct {
	openBracket  int // byte index of '['
	closeBracket int // byte index of matching ']'
	singleLine   bool
	closingLine  arrayLine // line containing ']' (multi-line only)
	entries      []arrayEntry
}

type arrayLine struct {
	lineStart int // start of line (byte index)
	indent    string
}

type arrayEntry struct {
	lineStart  int // start of the line carrying this entry
	lineEnd    int // start of the next line (or EOF) — used by removeLine
	valueStart int // byte index of opening quote
	valueEnd   int // byte index just past closing quote
	indent     string
}

// arrayStyle captures the formatting choices of an existing array.
type arrayStyle struct {
	indent        string // leading whitespace per entry line
	quote         byte   // ' or "
	trailingComma bool
}

func detectArrayStyle(data []byte, span *arraySpan) arrayStyle {
	style := arrayStyle{indent: "    ", quote: '"', trailingComma: true}
	if len(span.entries) == 0 {
		return style
	}
	first := span.entries[0]
	// Single-line arrays have empty per-entry indent (no \n before
	// the first quote). Keep the 4-space default so expansion lays
	// out cleanly; multi-line arrays copy whatever indent the user
	// already chose.
	if first.indent != "" {
		style.indent = first.indent
	}
	style.quote = data[first.valueStart]
	last := span.entries[len(span.entries)-1]
	// Trailing comma if a ',' appears between the last entry's
	// closing quote and the closing bracket.
	tail := data[last.valueEnd:span.closeBracket]
	style.trailingComma = bytes.ContainsRune(tail, ',')
	return style
}

var (
	tableHeaderRe = regexp.MustCompile(`(?m)^\s*\[\s*([A-Za-z0-9_.\-]+)\s*\]`)
	keyEqualsRe   = regexp.MustCompile(`^(\s*)([A-Za-z0-9_.\-]+|"[^"]+"|'[^']+')\s*=\s*`)
)

// findArraySpan locates `key = [ ... ]` inside the [table] section.
// Returns nil if the array isn't present in that section. Errors only
// on malformed TOML structure (unterminated array literal).
func findArraySpan(data []byte, table, key string) (*arraySpan, error) {
	sectionStart, sectionEnd := tableSection(data, table)
	if sectionStart < 0 {
		return nil, nil
	}

	// Scan lines within [sectionStart, sectionEnd) for `key = [`.
	cursor := sectionStart
	for cursor < sectionEnd {
		lineEnd := cursor + indexOrEnd(data[cursor:sectionEnd], '\n')
		lineEndIncl := lineEnd
		if lineEnd < len(data) {
			lineEndIncl = lineEnd + 1 // include the newline
		}
		line := data[cursor:lineEnd]

		if m := keyEqualsRe.FindSubmatchIndex(line); m != nil {
			rawKey := unquoteKey(string(line[m[4]:m[5]]))
			valueStart := cursor + m[1]
			if rawKey == key && valueStart < lineEnd && data[valueStart] == '[' {
				return scanArray(data, valueStart, cursor)
			}
		}
		cursor = lineEndIncl
	}
	return nil, nil
}

// tableSection returns [start, end) byte offsets covering the body of
// the named [table] section, exclusive of its header line. end is the
// byte offset of the next section header (or len(data) when the section
// runs to EOF).
func tableSection(data []byte, table string) (int, int) {
	headers := tableHeaderRe.FindAllSubmatchIndex(data, -1)
	for i, m := range headers {
		name := string(data[m[2]:m[3]])
		if name != table {
			continue
		}
		// Body starts after the header line's newline.
		bodyStart := m[1]
		if bodyStart < len(data) && data[bodyStart] == '\n' {
			bodyStart++
		}
		bodyEnd := len(data)
		if i+1 < len(headers) {
			bodyEnd = headers[i+1][0]
		}
		return bodyStart, bodyEnd
	}
	return -1, -1
}

// scanArray walks from openBracket forward, identifying entries and
// the matching closing bracket. Tracks single- and double-quoted
// string-literal state so a `]` inside a quoted string doesn't close
// the array.
//
// Does NOT handle TOML triple-quoted multiline strings (`"""..."""`).
// PEP 508 dep specs never contain those, but the editor is also not
// safe for arbitrary TOML arrays — only the dep-array shapes pensa
// edits via `pensa add`/`pensa remove`.
func scanArray(data []byte, openBracket, lineStart int) (*arraySpan, error) {
	span := &arraySpan{openBracket: openBracket}

	i := openBracket + 1
	depth := 1
	inString := false
	var stringQuote byte
	entryQuoteStart := -1
	entryLineStart := lineStart
	entryIndent := ""
	for i < len(data) && depth > 0 {
		b := data[i]
		switch {
		case inString:
			if b == stringQuote {
				inString = false
				span.entries = append(span.entries, arrayEntry{
					lineStart:  entryLineStart,
					lineEnd:    -1,
					valueStart: entryQuoteStart,
					valueEnd:   i + 1,
					indent:     entryIndent,
				})
				entryQuoteStart = -1
			}
		case b == '"' || b == '\'':
			inString = true
			stringQuote = b
			entryQuoteStart = i
		case b == '[':
			depth++
		case b == ']':
			depth--
			if depth == 0 {
				span.closeBracket = i
			}
		case b == '\n':
			entryLineStart = i + 1
			// Capture leading whitespace of the next entry.
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t') {
				j++
			}
			entryIndent = string(data[i+1 : j])
		}
		i++
	}

	if span.closeBracket == 0 {
		return nil, fmt.Errorf("unterminated array")
	}

	// Single-line if the entire array is on one source line.
	span.singleLine = !bytes.Contains(data[openBracket:span.closeBracket], []byte{'\n'})

	// Fill in lineEnd for each entry (start of next line or close-bracket-line).
	for idx := range span.entries {
		end := span.closeBracket
		if idx+1 < len(span.entries) {
			end = span.entries[idx+1].lineStart
		} else {
			// Find the end of the entry's line: next newline (inclusive)
			// or the closeBracket if no newline.
			search := span.entries[idx].valueEnd
			if nl := indexOrEnd(data[search:span.closeBracket], '\n'); nl < span.closeBracket-search {
				end = search + nl + 1
			}
		}
		span.entries[idx].lineEnd = end
	}

	// Closing-line indent (the line carrying the `]`).
	closeLineStart := span.closeBracket
	for closeLineStart > openBracket && data[closeLineStart-1] != '\n' {
		closeLineStart--
	}
	closeIndent := ""
	for j := closeLineStart; j < span.closeBracket && (data[j] == ' ' || data[j] == '\t'); j++ {
		closeIndent += string(data[j])
	}
	span.closingLine = arrayLine{lineStart: closeLineStart, indent: closeIndent}

	return span, nil
}

// expandArrayToMultiline rewrites a single-line array into the
// standard multi-line shape so subsequent edits stay legible. Uses
// the detected style (indent, quotes are already preserved by
// copying the original entry bytes verbatim).
func expandArrayToMultiline(data []byte, span *arraySpan) ([]byte, *arraySpan, error) {
	style := detectArrayStyle(data, span)
	var b bytes.Buffer
	b.Write(data[:span.openBracket+1])
	b.WriteByte('\n')
	for _, e := range span.entries {
		b.WriteString(style.indent)
		b.Write(data[e.valueStart:e.valueEnd])
		b.WriteString(",\n")
	}
	b.WriteByte(']')
	b.Write(data[span.closeBracket+1:])

	newSpan, err := scanArray(b.Bytes(), span.openBracket, span.openBracket)
	if err != nil {
		return nil, nil, err
	}
	return b.Bytes(), newSpan, nil
}

// insertNewEntry appends `entry` immediately before the closing `]`,
// matching the existing array's indent and quote style.
//
// When the existing array has no trailing comma on its last entry,
// we MUST add one — that entry is no longer last after insertion,
// and TOML rejects two array entries on consecutive lines without
// a separating comma. The "no trailing comma" style is preserved
// only on the new final entry.
func insertNewEntry(data []byte, span *arraySpan, style arrayStyle, entry string) []byte {
	insertPos := span.closingLine.lineStart
	var b bytes.Buffer
	if !style.trailingComma && len(span.entries) > 0 {
		last := span.entries[len(span.entries)-1]
		b.Write(data[:last.valueEnd])
		b.WriteByte(',')
		b.Write(data[last.valueEnd:insertPos])
	} else {
		b.Write(data[:insertPos])
	}
	b.WriteString(style.indent)
	b.WriteString(quoteEntry(entry, style.quote))
	if style.trailingComma {
		b.WriteByte(',')
	}
	b.WriteByte('\n')
	b.Write(data[insertPos:])
	return b.Bytes()
}

// appendArrayKey adds a brand-new `key = ["entry"]` line at the end
// of the [table] section.
func appendArrayKey(data []byte, table, key, entry string) ([]byte, error) {
	_, sectionEnd := tableSection(data, table)
	if sectionEnd < 0 {
		return nil, fmt.Errorf("table [%s] not found", table)
	}
	insertion := fmt.Sprintf("%s = [\n    %q,\n]\n", key, entry)
	insertAt := sectionEnd
	// Avoid a blank line inserted between the new key and the next table.
	if insertAt > 0 && insertAt < len(data) && data[insertAt-1] != '\n' {
		insertion = "\n" + insertion
	}
	var b bytes.Buffer
	b.Write(data[:insertAt])
	b.WriteString(insertion)
	b.Write(data[insertAt:])
	return b.Bytes(), nil
}

// removeLine deletes the half-open byte range [start, end) covering
// exactly one source line.
func removeLine(data []byte, start, end int) []byte {
	var b bytes.Buffer
	b.Write(data[:start])
	b.Write(data[end:])
	return b.Bytes()
}

// spliceBytes replaces data[from:to] with replacement.
func spliceBytes(data []byte, from, to int, replacement []byte) []byte {
	var b bytes.Buffer
	b.Write(data[:from])
	b.Write(replacement)
	b.Write(data[to:])
	return b.Bytes()
}

// quoteEntry wraps text in q quotes. TOML allows " and '; we don't
// attempt escaping because dep strings (PEP 508) never contain
// either character.
func quoteEntry(text string, q byte) string {
	return string(q) + text + string(q)
}

// depEntryName extracts the package name from a quoted dep string.
// `"requests[security]>=2.28"` returns `requests`.
func depEntryName(quoted string) string {
	if len(quoted) < 2 {
		return ""
	}
	q := quoted[0]
	if q != '"' && q != '\'' {
		return ""
	}
	closing := strings.LastIndexByte(quoted, q)
	if closing <= 0 {
		return ""
	}
	body := quoted[1:closing]
	for i, r := range body {
		if r == '[' || r == ' ' || r == '<' || r == '>' || r == '=' || r == '!' || r == '~' || r == ';' || r == '@' {
			return strings.TrimSpace(body[:i])
		}
	}
	return strings.TrimSpace(body)
}

func unquoteKey(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func indexOrEnd(b []byte, ch byte) int {
	if i := bytes.IndexByte(b, ch); i >= 0 {
		return i
	}
	return len(b)
}
