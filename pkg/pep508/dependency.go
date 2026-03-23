package pep508

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juanbzz/pensa/pkg/version"
)

// Dependency represents a parsed PEP 508 dependency specifier.
type Dependency struct {
	Name       string              // normalized package name
	Extras     []string            // optional extras like [security, tests]
	Constraint version.Constraint  // nil means any version
	URL        string              // for URL-based deps (@ https://...)
	Markers    Marker              // nil means unconditional
}

var nameNormalizer = regexp.MustCompile(`[-_.]+`)

// NormalizeName normalizes a Python package name per PEP 503.
func NormalizeName(name string) string {
	return nameNormalizer.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
}

// Parse parses a PEP 508 dependency specifier string.
func Parse(s string) (Dependency, error) {
	p := &depParser{input: strings.TrimSpace(s)}
	return p.parse()
}

type depParser struct {
	input string
	pos   int
}

func (p *depParser) remaining() string {
	if p.pos >= len(p.input) {
		return ""
	}
	return p.input[p.pos:]
}

func (p *depParser) skipWS() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *depParser) parse() (Dependency, error) {
	var dep Dependency

	// 1. Parse package name.
	name := p.consumeName()
	if name == "" {
		return dep, fmt.Errorf("pep508: empty package name in %q", p.input)
	}
	dep.Name = NormalizeName(name)

	p.skipWS()

	// 2. Parse extras: [extra1, extra2]
	if p.pos < len(p.input) && p.input[p.pos] == '[' {
		extras, err := p.consumeExtras()
		if err != nil {
			return dep, err
		}
		dep.Extras = extras
	}

	p.skipWS()

	// 3. URL or version specifier.
	if p.pos < len(p.input) && p.input[p.pos] == '@' {
		p.pos++ // consume '@'
		p.skipWS()
		url := p.consumeUntil(';')
		dep.URL = strings.TrimSpace(url)
	} else {
		// Version specifier: everything until ';' or end.
		verStr := strings.TrimSpace(p.consumeUntil(';'))
		if verStr != "" {
			// Strip surrounding parens if present: (>=1.0,<2.0)
			if len(verStr) >= 2 && verStr[0] == '(' && verStr[len(verStr)-1] == ')' {
				verStr = verStr[1 : len(verStr)-1]
			}
			c, err := version.ParseConstraint(verStr)
			if err != nil {
				return dep, fmt.Errorf("pep508: invalid version specifier in %q: %w", p.input, err)
			}
			dep.Constraint = c
		}
	}

	// 4. Markers: ; marker_expr
	p.skipWS()
	if p.pos < len(p.input) && p.input[p.pos] == ';' {
		p.pos++ // consume ';'
		markerStr := strings.TrimSpace(p.remaining())
		if markerStr != "" {
			m, err := ParseMarker(markerStr)
			if err != nil {
				return dep, fmt.Errorf("pep508: invalid marker in %q: %w", p.input, err)
			}
			dep.Markers = m
		}
		p.pos = len(p.input)
	}

	return dep, nil
}

func (p *depParser) consumeName() string {
	start := p.pos
	for p.pos < len(p.input) && isNameChar(p.input[p.pos]) {
		p.pos++
	}
	return p.input[start:p.pos]
}

func isNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.'
}

func (p *depParser) consumeExtras() ([]string, error) {
	p.pos++ // consume '['
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != ']' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("pep508: unclosed extras bracket in %q", p.input)
	}
	extrasStr := p.input[start:p.pos]
	p.pos++ // consume ']'

	var extras []string
	for _, e := range strings.Split(extrasStr, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			extras = append(extras, NormalizeName(e))
		}
	}
	return extras, nil
}

func (p *depParser) consumeUntil(stop byte) string {
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != stop {
		p.pos++
	}
	return p.input[start:p.pos]
}
