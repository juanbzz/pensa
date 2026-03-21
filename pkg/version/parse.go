package version

import (
	"fmt"
	"regexp"
	"strings"
)

// ParseConstraint parses a version constraint string into a Constraint.
// Supports PEP 440 operators (==, !=, <, <=, >, >=, ~=, ===),
// Poetry operators (^, ~), wildcards (==1.0.*), and combinations (AND with comma, OR with ||).
func ParseConstraint(s string) (Constraint, error) {
	s = strings.TrimSpace(s)

	if s == "" || s == "*" {
		return AnyConstraint(), nil
	}

	// Split on OR (|| or |).
	orParts := splitOr(s)
	if len(orParts) > 1 {
		var orConstraints []Constraint
		for _, part := range orParts {
			c, err := ParseConstraint(part)
			if err != nil {
				return nil, err
			}
			orConstraints = append(orConstraints, c)
		}
		return NewUnion(orConstraints...), nil
	}

	// Split on AND (comma-separated). Tolerate trailing commas.
	andParts := splitAnd(s)
	if len(andParts) > 1 {
		result := Constraint(AnyConstraint())
		for _, part := range andParts {
			c, err := parseSingle(part)
			if err != nil {
				return nil, err
			}
			result = result.Intersect(c)
		}
		return result, nil
	}

	return parseSingle(s)
}

func splitOr(s string) []string {
	parts := regexp.MustCompile(`\s*\|\|?\s*`).Split(s, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func splitAnd(s string) []string {
	s = strings.TrimRight(s, ", ")
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Regex patterns matching Poetry's constraint patterns.
var (
	caretRe     = regexp.MustCompile(`(?i)^\^\s*(.+)$`)
	tildeRe     = regexp.MustCompile(`(?i)^~\s*(.+)$`)
	tildePEP440 = regexp.MustCompile(`(?i)^~=\s*(.+)$`)
	xConstraint = regexp.MustCompile(`(?i)^(?P<op>!=|==)?\s*v?(?P<version>[0-9]+(?:\.[0-9]+)*)\.\*$`)
	basicRe      = regexp.MustCompile(`(?i)^(?P<op><>|!=|>=?|<=?|==?|===)?\s*(?P<version>.+?)(?P<wildcard>\.\*)?$`)
	bareWildcard = regexp.MustCompile(`(?i)^v?[xX*](\.[xX*])*$`)
)

func parseSingle(s string) (Constraint, error) {
	s = strings.TrimSpace(s)

	// Bare wildcard.
	if bareWildcard.MatchString(s) {
		return AnyConstraint(), nil
	}

	// Arbitrary equality: ===value
	if strings.HasPrefix(s, "===") {
		return &arbitraryConstraint{raw: strings.TrimSpace(s[3:])}, nil
	}

	// Caret range: ^V
	if m := caretRe.FindStringSubmatch(s); m != nil {
		v, err := Parse(m[1])
		if err != nil {
			return nil, fmt.Errorf("parse constraint %q: %w", s, err)
		}
		high := v.NextBreaking()
		return NewRange(&v, &high, true, false), nil
	}

	// PEP 440 compatible release: ~=V
	if m := tildePEP440.FindStringSubmatch(s); m != nil {
		v, err := Parse(m[1])
		if err != nil {
			return nil, fmt.Errorf("parse constraint %q: %w", s, err)
		}
		var high Version
		if len(v.release) <= 2 {
			high = v.NextMajor()
		} else {
			high = v.NextMinor()
		}
		return NewRange(&v, &high, true, false), nil
	}

	// Poetry tilde range: ~V (not ~=)
	if m := tildeRe.FindStringSubmatch(s); m != nil {
		v, err := Parse(m[1])
		if err != nil {
			return nil, fmt.Errorf("parse constraint %q: %w", s, err)
		}
		var high Version
		if len(v.release) == 1 {
			high = v.NextMajor()
		} else {
			high = v.NextMinor()
		}
		return NewRange(&v, &high, true, false), nil
	}

	// Wildcard: ==V.* or !=V.*
	if m := xConstraint.FindStringSubmatch(s); m != nil {
		op := namedGroupMatch(m, xConstraint, "op")
		verStr := namedGroupMatch(m, xConstraint, "version")
		v, err := Parse(verStr)
		if err != nil {
			return nil, fmt.Errorf("parse constraint %q: %w", s, err)
		}
		return makeWildcardRange(v, op == "!="), nil
	}

	// Basic comparator: op + version.
	if m := basicRe.FindStringSubmatch(s); m != nil {
		op := namedGroupMatch(m, basicRe, "op")
		verStr := namedGroupMatch(m, basicRe, "version")
		wildcard := namedGroupMatch(m, basicRe, "wildcard")

		if verStr == "" {
			return nil, fmt.Errorf("parse constraint %q: empty version", s)
		}

		// Arbitrary equality.
		if op == "===" {
			return &arbitraryConstraint{raw: verStr}, nil
		}

		v, err := Parse(verStr)
		if err != nil {
			return nil, fmt.Errorf("parse constraint %q: %w", s, err)
		}

		// Wildcard with basic operator.
		if wildcard == ".*" {
			return makeWildcardRange(v, op == "!=" || op == "<>"), nil
		}

		switch op {
		case "<":
			return NewRange(nil, &v, false, false), nil
		case "<=":
			return NewRange(nil, &v, false, true), nil
		case ">":
			return NewRange(&v, nil, false, false), nil
		case ">=":
			return NewRange(&v, nil, true, false), nil
		case "!=", "<>":
			return NewUnion(
				NewRange(nil, &v, false, false),
				NewRange(&v, nil, false, false),
			), nil
		case "==", "":
			return ExactVersion(v), nil
		default:
			return nil, fmt.Errorf("parse constraint %q: unknown operator %q", s, op)
		}
	}

	return nil, fmt.Errorf("parse constraint: invalid constraint %q", s)
}

func namedGroupMatch(match []string, re *regexp.Regexp, name string) string {
	for i, n := range re.SubexpNames() {
		if n == name && i < len(match) {
			return match[i]
		}
	}
	return ""
}

func makeWildcardRange(v Version, invert bool) Constraint {
	low := v.FirstDevRelease()
	high := v.NextStable()
	if !high.IsDevRelease() {
		high = high.FirstDevRelease()
	}
	result := NewRange(&low, &high, true, false)
	if invert {
		return AnyConstraint().Difference(result)
	}
	return result
}

// arbitraryConstraint handles the === operator (raw string equality).
type arbitraryConstraint struct {
	raw string
}

func (a *arbitraryConstraint) IsEmpty() bool              { return false }
func (a *arbitraryConstraint) IsAny() bool                { return false }
func (a *arbitraryConstraint) Allows(v Version) bool      { return v.String() == a.raw }
func (a *arbitraryConstraint) AllowsAll(Constraint) bool  { return false }
func (a *arbitraryConstraint) AllowsAny(other Constraint) bool {
	if o, ok := other.(*arbitraryConstraint); ok {
		return a.raw == o.raw
	}
	return false
}
func (a *arbitraryConstraint) Intersect(other Constraint) Constraint {
	if other.IsAny() {
		return a
	}
	if o, ok := other.(*arbitraryConstraint); ok && a.raw == o.raw {
		return a
	}
	return empty
}
func (a *arbitraryConstraint) Union(other Constraint) Constraint  { return NewUnion(a, other) }
func (a *arbitraryConstraint) Difference(other Constraint) Constraint {
	if other.Allows(Version{}) && a.Allows(Version{}) {
		return empty
	}
	return a
}
func (a *arbitraryConstraint) String() string { return "===" + a.raw }
