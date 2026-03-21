package pep508

import (
	"fmt"
	"strings"

	"github.com/juanbzz/goetry/pkg/version"
)

// Marker represents a PEP 508 environment marker expression.
type Marker interface {
	Evaluate(env Environment) bool
	String() string
}

// Environment holds the values for PEP 508 marker evaluation.
type Environment struct {
	PythonVersion                string // "3.11"
	PythonFullVersion            string // "3.11.4"
	OSName                       string // "posix"
	SysPlatform                  string // "linux", "darwin", "win32"
	PlatformRelease              string
	PlatformSystem               string // "Linux", "Darwin", "Windows"
	PlatformVersion              string
	PlatformMachine              string // "x86_64", "arm64"
	PlatformPythonImplementation string // "CPython"
	ImplementationName           string // "cpython"
	ImplementationVersion        string // "3.11.4"
	Extra                        string
}

func (e Environment) lookup(name string) (string, bool) {
	// Support legacy aliases.
	switch name {
	case "os.name":
		name = "os_name"
	case "sys.platform":
		name = "sys_platform"
	case "platform.version":
		name = "platform_version"
	case "platform.machine":
		name = "platform_machine"
	case "platform.python_implementation", "python_implementation":
		name = "platform_python_implementation"
	}

	switch name {
	case "python_version":
		return e.PythonVersion, true
	case "python_full_version":
		return e.PythonFullVersion, true
	case "os_name":
		return e.OSName, true
	case "sys_platform":
		return e.SysPlatform, true
	case "platform_release":
		return e.PlatformRelease, true
	case "platform_system":
		return e.PlatformSystem, true
	case "platform_version":
		return e.PlatformVersion, true
	case "platform_machine":
		return e.PlatformMachine, true
	case "platform_python_implementation":
		return e.PlatformPythonImplementation, true
	case "implementation_name":
		return e.ImplementationName, true
	case "implementation_version":
		return e.ImplementationVersion, true
	case "extra":
		return e.Extra, true
	}
	return "", false
}

// pythonVersionMarkers are the marker names where PEP 440 version comparison applies.
var pythonVersionMarkers = map[string]bool{
	"python_version":        true,
	"python_full_version":   true,
	"implementation_version": true,
}

// --- Concrete marker types ---

// AnyMarker always evaluates to true (no markers present).
type AnyMarker struct{}

func (AnyMarker) Evaluate(Environment) bool { return true }
func (AnyMarker) String() string            { return "" }

// CompareMarker represents a single comparison like `python_version >= "3.8"`.
type CompareMarker struct {
	Var   string
	Op    string
	Value string
}

func (m *CompareMarker) Evaluate(env Environment) bool {
	envVal, ok := env.lookup(m.Var)
	if !ok {
		return false
	}
	return evalCompare(m.Var, envVal, m.Op, m.Value)
}

func (m *CompareMarker) String() string {
	return fmt.Sprintf("%s %s %q", m.Var, m.Op, m.Value)
}

// AndMarker represents a logical AND of two markers.
type AndMarker struct {
	Left, Right Marker
}

func (m *AndMarker) Evaluate(env Environment) bool {
	return m.Left.Evaluate(env) && m.Right.Evaluate(env)
}

func (m *AndMarker) String() string {
	return m.Left.String() + " and " + m.Right.String()
}

// OrMarker represents a logical OR of two markers.
type OrMarker struct {
	Left, Right Marker
}

func (m *OrMarker) Evaluate(env Environment) bool {
	return m.Left.Evaluate(env) || m.Right.Evaluate(env)
}

func (m *OrMarker) String() string {
	return m.Left.String() + " or " + m.Right.String()
}

// --- Comparison evaluation ---

func evalCompare(varName, envVal, op, specVal string) bool {
	switch op {
	case "in":
		return strings.Contains(envVal, specVal)
	case "not in":
		return !strings.Contains(envVal, specVal)
	}

	// For python version markers, use PEP 440 comparison.
	if pythonVersionMarkers[varName] {
		return evalVersionCompare(envVal, op, specVal)
	}

	// String comparison for all others.
	return evalStringCompare(envVal, op, specVal)
}

func evalVersionCompare(envVal, op, specVal string) bool {
	ev, err1 := version.Parse(envVal)
	sv, err2 := version.Parse(specVal)
	if err1 != nil || err2 != nil {
		return evalStringCompare(envVal, op, specVal)
	}

	c := version.Compare(ev, sv)
	switch op {
	case "==":
		return c == 0
	case "!=":
		return c != 0
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	case "~=":
		// Compatible release: >= specVal and == specVal.* (same major.minor)
		if c < 0 {
			return false
		}
		return ev.Major() == sv.Major() && (len(sv.Release()) < 2 || ev.Minor() == sv.Minor())
	case "===":
		return envVal == specVal
	}
	return false
}

func evalStringCompare(a, op, b string) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "===":
		return a == b
	}
	return false
}

// --- Marker parsing (recursive descent) ---

// ParseMarker parses a PEP 508 marker expression string.
func ParseMarker(s string) (Marker, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return AnyMarker{}, nil
	}
	p := &markerParser{input: s}
	m, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected trailing text in marker: %q", p.input[p.pos:])
	}
	return m, nil
}

type markerParser struct {
	input string
	pos   int
}

func (p *markerParser) skipWS() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *markerParser) peek() byte {
	p.skipWS()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *markerParser) consumeWord() string {
	p.skipWS()
	start := p.pos
	for p.pos < len(p.input) && isWordChar(p.input[p.pos]) {
		p.pos++
	}
	return p.input[start:p.pos]
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.'
}

func (p *markerParser) parseOr() (Marker, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if p.pos+2 <= len(p.input) && p.input[p.pos:p.pos+2] == "or" && !isWordChar(p.input[min(p.pos+2, len(p.input)-1)]) {
			p.pos += 2
			right, err := p.parseAnd()
			if err != nil {
				return nil, err
			}
			left = &OrMarker{Left: left, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (p *markerParser) parseAnd() (Marker, error) {
	left, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if p.pos+3 <= len(p.input) && p.input[p.pos:p.pos+3] == "and" && !isWordChar(p.input[min(p.pos+3, len(p.input)-1)]) {
			p.pos += 3
			right, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			left = &AndMarker{Left: left, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (p *markerParser) parseExpr() (Marker, error) {
	p.skipWS()
	if p.pos < len(p.input) && p.input[p.pos] == '(' {
		p.pos++ // consume '('
		m, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, fmt.Errorf("expected ')' in marker at pos %d", p.pos)
		}
		p.pos++ // consume ')'
		return m, nil
	}

	// Parse: marker_var op marker_value  OR  marker_value op marker_var
	left := p.parseValue()
	op, err := p.parseOp()
	if err != nil {
		return nil, err
	}
	right := p.parseValue()

	// Determine which side is the variable and which is the literal.
	varName, value := left, right
	if isQuoted(left) {
		varName, value = right, left
	}
	value = unquote(value)

	return &CompareMarker{Var: varName, Op: op, Value: value}, nil
}

func (p *markerParser) parseValue() string {
	p.skipWS()
	if p.pos >= len(p.input) {
		return ""
	}

	// Quoted string.
	if p.input[p.pos] == '\'' || p.input[p.pos] == '"' {
		quote := p.input[p.pos]
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != quote {
			p.pos++
		}
		val := p.input[start:p.pos]
		if p.pos < len(p.input) {
			p.pos++ // consume closing quote
		}
		return string(quote) + val + string(quote)
	}

	// Unquoted identifier (marker variable name).
	return p.consumeWord()
}

func (p *markerParser) parseOp() (string, error) {
	p.skipWS()
	if p.pos >= len(p.input) {
		return "", fmt.Errorf("expected operator at pos %d", p.pos)
	}

	// Multi-char operators: not in, in, ===, ~=, ==, !=, >=, <=
	remaining := p.input[p.pos:]

	// "not in"
	if strings.HasPrefix(remaining, "not") {
		saved := p.pos
		p.pos += 3
		p.skipWS()
		if p.pos+2 <= len(p.input) && p.input[p.pos:p.pos+2] == "in" {
			p.pos += 2
			return "not in", nil
		}
		p.pos = saved
	}

	// "in"
	if strings.HasPrefix(remaining, "in") && (len(remaining) == 2 || !isWordChar(remaining[2])) {
		p.pos += 2
		return "in", nil
	}

	// Three-char: ===
	if len(remaining) >= 3 && remaining[:3] == "===" {
		p.pos += 3
		return "===", nil
	}

	// Two-char: ~=, ==, !=, >=, <=
	if len(remaining) >= 2 {
		two := remaining[:2]
		switch two {
		case "~=", "==", "!=", ">=", "<=":
			p.pos += 2
			return two, nil
		}
	}

	// Single-char: <, >
	switch remaining[0] {
	case '<', '>':
		p.pos++
		return string(remaining[0]), nil
	}

	return "", fmt.Errorf("unknown operator at pos %d: %q", p.pos, remaining[:min(10, len(remaining))])
}

func isQuoted(s string) bool {
	return len(s) >= 2 && (s[0] == '\'' || s[0] == '"')
}

func unquote(s string) string {
	if isQuoted(s) {
		return s[1 : len(s)-1]
	}
	return s
}
