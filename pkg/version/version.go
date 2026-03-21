package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PreKind represents the type of pre-release.
type PreKind int

const (
	Alpha PreKind = iota + 1
	Beta
	RC
)

func (k PreKind) String() string {
	switch k {
	case Alpha:
		return "a"
	case Beta:
		return "b"
	case RC:
		return "rc"
	default:
		return ""
	}
}

// Pre represents a pre-release segment.
type Pre struct {
	Kind PreKind
	Num  int
}

// Version represents a PEP 440 version.
// All fields are unexported; use accessors or Parse to construct.
type Version struct {
	epoch   int
	release []int
	pre     *Pre
	post    *int
	dev     *int
	local   []string
}

// Accessors

func (v Version) Epoch() int        { return v.epoch }
func (v Version) Release() []int    { return append([]int(nil), v.release...) }
func (v Version) Pre() *Pre         { return v.pre }
func (v Version) Post() *int        { return v.post }
func (v Version) Dev() *int         { return v.dev }
func (v Version) Local() []string   { return v.local }

func (v Version) Major() int {
	if len(v.release) > 0 {
		return v.release[0]
	}
	return 0
}

func (v Version) Minor() int {
	if len(v.release) > 1 {
		return v.release[1]
	}
	return 0
}

func (v Version) Patch() int {
	if len(v.release) > 2 {
		return v.release[2]
	}
	return 0
}

func (v Version) IsPreRelease() bool  { return v.pre != nil }
func (v Version) IsPostRelease() bool { return v.post != nil }
func (v Version) IsDevRelease() bool  { return v.dev != nil }
func (v Version) IsLocal() bool       { return v.local != nil }
func (v Version) IsStable() bool      { return !v.IsPreRelease() && !v.IsDevRelease() }

// String returns the canonical normalized form.
func (v Version) String() string {
	var b strings.Builder

	if v.epoch != 0 {
		fmt.Fprintf(&b, "%d!", v.epoch)
	}

	for i, n := range v.release {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(strconv.Itoa(n))
	}

	if v.pre != nil {
		fmt.Fprintf(&b, "%s%d", v.pre.Kind, v.pre.Num)
	}

	if v.post != nil {
		fmt.Fprintf(&b, ".post%d", *v.post)
	}

	if v.dev != nil {
		fmt.Fprintf(&b, ".dev%d", *v.dev)
	}

	if v.local != nil {
		b.WriteByte('+')
		b.WriteString(strings.Join(v.local, "."))
	}

	return b.String()
}

// Mutation helpers — all return new Version values.

func (v Version) NextMajor() Version {
	rel := make([]int, len(v.release))
	rel[0] = v.Major() + 1
	return Version{epoch: v.epoch, release: rel}
}

func (v Version) NextMinor() Version {
	rel := make([]int, max(len(v.release), 2))
	rel[0] = v.Major()
	rel[1] = v.Minor() + 1
	return Version{epoch: v.epoch, release: rel}
}

func (v Version) NextPatch() Version {
	rel := make([]int, max(len(v.release), 3))
	rel[0] = v.Major()
	rel[1] = v.Minor()
	rel[2] = v.Patch() + 1
	return Version{epoch: v.epoch, release: rel}
}

func (v Version) NextBreaking() Version {
	if v.Major() > 0 || len(v.release) < 2 {
		return v.NextMajor()
	}
	if v.Minor() > 0 || len(v.release) < 3 {
		return v.NextMinor()
	}
	return v.NextPatch()
}

func (v Version) NextStable() Version {
	if v.IsStable() {
		// Increment the last release segment.
		rel := append([]int(nil), v.release...)
		rel[len(rel)-1]++
		return Version{epoch: v.epoch, release: rel}
	}
	return Version{epoch: v.epoch, release: append([]int(nil), v.release...)}
}

func (v Version) FirstDevRelease() Version {
	dev := 0
	return Version{
		epoch:   v.epoch,
		release: append([]int(nil), v.release...),
		pre:     v.pre,
		post:    v.post,
		dev:     &dev,
	}
}

func (v Version) WithoutLocal() Version {
	return Version{
		epoch:   v.epoch,
		release: append([]int(nil), v.release...),
		pre:     v.pre,
		post:    v.post,
		dev:     v.dev,
	}
}

func (v Version) WithoutPostRelease() Version {
	if !v.IsPostRelease() {
		return v
	}
	return Version{
		epoch:   v.epoch,
		release: append([]int(nil), v.release...),
		pre:     v.pre,
	}
}

func (v Version) WithoutDevRelease() Version {
	if !v.IsDevRelease() {
		return v
	}
	return Version{
		epoch:   v.epoch,
		release: append([]int(nil), v.release...),
		pre:     v.pre,
		post:    v.post,
	}
}

// Compare returns -1, 0, or +1 comparing a and b per PEP 440.
func Compare(a, b Version) int {
	// 1. Epoch
	if c := cmpInt(a.epoch, b.epoch); c != 0 {
		return c
	}

	// 2. Release segments (trailing zeros stripped)
	if c := cmpRelease(a.release, b.release); c != 0 {
		return c
	}

	// 3. Pre-release key
	if c := cmpInt(preKey(a), preKey(b)); c != 0 {
		return c
	}
	if a.pre != nil && b.pre != nil {
		if c := cmpInt(int(a.pre.Kind), int(b.pre.Kind)); c != 0 {
			return c
		}
		if c := cmpInt(a.pre.Num, b.pre.Num); c != 0 {
			return c
		}
	}

	// 4. Post-release key
	if c := cmpInt(postKey(a), postKey(b)); c != 0 {
		return c
	}
	if a.post != nil && b.post != nil {
		if c := cmpInt(*a.post, *b.post); c != 0 {
			return c
		}
	}

	// 5. Dev-release key
	if c := cmpInt(devKey(a), devKey(b)); c != 0 {
		return c
	}
	if a.dev != nil && b.dev != nil {
		if c := cmpInt(*a.dev, *b.dev); c != 0 {
			return c
		}
	}

	// 6. Local version
	return cmpLocal(a.local, b.local)
}

const (
	negInf = -1000
	posInf = 1000
)

// preKey: dev-only (no pre, no post) → negInf; no pre → posInf; else kind value.
func preKey(v Version) int {
	if v.pre == nil && v.post == nil && v.dev != nil {
		return negInf
	}
	if v.pre == nil {
		return posInf
	}
	return 0 // actual comparison handled by kind+num
}

// postKey: no post → negInf; else 0 (actual comparison by post num).
func postKey(v Version) int {
	if v.post == nil {
		return negInf
	}
	return 0
}

// devKey: no dev → posInf; else 0 (actual comparison by dev num).
func devKey(v Version) int {
	if v.dev == nil {
		return posInf
	}
	return 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func cmpRelease(a, b []int) int {
	a = stripTrailingZeros(a)
	b = stripTrailingZeros(b)

	n := max(len(a), len(b))
	for i := range n {
		ai, bi := 0, 0
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		if c := cmpInt(ai, bi); c != 0 {
			return c
		}
	}
	return 0
}

func stripTrailingZeros(s []int) []int {
	i := len(s)
	for i > 0 && s[i-1] == 0 {
		i--
	}
	return s[:i]
}

func cmpLocal(a, b []string) int {
	if a == nil && b == nil {
		return 0
	}
	// No local sorts before local.
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	n := max(len(a), len(b))
	for i := range n {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		c := cmpLocalSegment(a[i], b[i])
		if c != 0 {
			return c
		}
	}
	return 0
}

func cmpLocalSegment(a, b string) int {
	ai, aIsNum := strconv.Atoi(a)
	bi, bIsNum := strconv.Atoi(b)

	switch {
	case aIsNum == nil && bIsNum == nil:
		return cmpInt(ai, bi)
	case aIsNum == nil:
		return 1 // numeric > alpha
	case bIsNum == nil:
		return -1
	default:
		return cmpStr(strings.ToLower(a), strings.ToLower(b))
	}
}

func cmpStr(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Parsing

// versionRegex matches PEP 440 versions including non-normalized forms.
// Based on packaging.version.VERSION_PATTERN.
var versionRegex = regexp.MustCompile(`(?i)` +
	`^\s*v?` +
	`(?:(?P<epoch>[0-9]+)!)?` +
	`(?P<release>[0-9]+(?:\.[0-9]+)*)` +
	`(?P<pre>[-_.]?(?P<pre_l>alpha|beta|preview|pre|a|b|c|rc)[-_.]?(?P<pre_n>[0-9]*))?` +
	`(?P<post>[-_.]?(?:(?P<post_l>post|rev|r)[-_.]?(?P<post_n>[0-9]*))|(?:-(?P<post_n2>[0-9]+)))?` +
	`(?P<dev>[-_.]?(?P<dev_l>dev)[-_.]?(?P<dev_n>[0-9]*))?` +
	`(?:\+(?P<local>[a-z0-9]+(?:[-_.][a-z0-9]+)*))?` +
	`\s*$`)

var versionGroupNames = versionRegex.SubexpNames()

func namedGroup(match []string, name string) string {
	for i, n := range versionGroupNames {
		if n == name && i < len(match) {
			return match[i]
		}
	}
	return ""
}

// Parse parses a PEP 440 version string, applying normalization.
func Parse(s string) (Version, error) {
	match := versionRegex.FindStringSubmatch(s)
	if match == nil {
		return Version{}, fmt.Errorf("invalid PEP 440 version: %q", s)
	}

	var v Version

	// Epoch
	if e := namedGroup(match, "epoch"); e != "" {
		n, _ := strconv.Atoi(e)
		v.epoch = n
	}

	// Release
	for _, part := range strings.Split(namedGroup(match, "release"), ".") {
		n, _ := strconv.Atoi(part)
		v.release = append(v.release, n)
	}

	// Pre-release
	if preL := namedGroup(match, "pre_l"); preL != "" {
		kind := normalizePreKind(strings.ToLower(preL))
		num := 0
		if preN := namedGroup(match, "pre_n"); preN != "" {
			num, _ = strconv.Atoi(preN)
		}
		v.pre = &Pre{Kind: kind, Num: num}
	}

	// Post-release
	if namedGroup(match, "post") != "" {
		num := 0
		if postN := namedGroup(match, "post_n"); postN != "" {
			num, _ = strconv.Atoi(postN)
		} else if postN2 := namedGroup(match, "post_n2"); postN2 != "" {
			num, _ = strconv.Atoi(postN2)
		}
		v.post = &num
	}

	// Dev-release
	if namedGroup(match, "dev") != "" {
		num := 0
		if devN := namedGroup(match, "dev_n"); devN != "" {
			num, _ = strconv.Atoi(devN)
		}
		v.dev = &num
	}

	// Local
	if loc := namedGroup(match, "local"); loc != "" {
		sep := regexp.MustCompile(`[-_.]`)
		parts := sep.Split(loc, -1)
		v.local = make([]string, len(parts))
		for i, p := range parts {
			v.local[i] = strings.ToLower(p)
		}
	}

	return v, nil
}

var preKindNormalization = map[string]PreKind{
	"a":       Alpha,
	"alpha":   Alpha,
	"b":       Beta,
	"beta":    Beta,
	"rc":      RC,
	"c":       RC,
	"pre":     RC,
	"preview": RC,
}

func normalizePreKind(s string) PreKind {
	if k, ok := preKindNormalization[s]; ok {
		return k
	}
	return Alpha
}
