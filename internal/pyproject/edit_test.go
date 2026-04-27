package pyproject

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// EditDepArray's contract is "preserve everything you don't touch."
// Tests run the editor against representative pyproject shapes and
// assert that bytes outside the targeted array span come back
// unchanged, while the array itself reflects the requested edit in
// the file's existing style (indent, quote, trailing comma).

const samplePyproject = `[project]
name = "demo"
version = "0.1.0"
authors = [{name = "Jane Doe", email = "jane@example.com"}]
dependencies = [
    "requests>=2.28",
    "rich>=13.0",
]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`

func TestEditDepArray_AddPreservesEverythingElse(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(samplePyproject), "project", "dependencies", EditAdd, "click>=8.0", "click")
	assert.NoErr(err)

	got := string(out)
	// Each non-deps line in the source must round-trip unchanged.
	for _, line := range []string{
		`name = "demo"`,
		`version = "0.1.0"`,
		`authors = [{name = "Jane Doe", email = "jane@example.com"}]`,
		`requires = ["hatchling"]`,
		`build-backend = "hatchling.build"`,
	} {
		assert.True(strings.Contains(got, line))
	}
	// New entry appended in matching style (4-space indent, double
	// quotes, trailing comma).
	assert.True(strings.Contains(got, "    \"click>=8.0\",\n"))
}

func TestEditDepArray_AddReplacesExistingByName(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(samplePyproject), "project", "dependencies", EditAdd, "requests>=2.31", "requests")
	assert.NoErr(err)

	got := string(out)
	assert.True(strings.Contains(got, `"requests>=2.31"`))
	assert.True(!strings.Contains(got, `"requests>=2.28"`))
	// Should NOT have appended a duplicate.
	assert.Equal(strings.Count(got, "requests"), 1)
}

func TestEditDepArray_RemoveDeletesOnlyTheTargetLine(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(samplePyproject), "project", "dependencies", EditRemove, "", "rich")
	assert.NoErr(err)

	got := string(out)
	assert.True(!strings.Contains(got, "rich"))
	assert.True(strings.Contains(got, `"requests>=2.28",`))
	// authors line must survive intact.
	assert.True(strings.Contains(got, `authors = [{name = "Jane Doe", email = "jane@example.com"}]`))
}

func TestEditDepArray_RemoveMissingNameIsNoOp(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(samplePyproject), "project", "dependencies", EditRemove, "", "absent")
	assert.NoErr(err)
	assert.Equal(string(out), samplePyproject)
}

const singleQuotedSample = `[project]
name = 'demo'
dependencies = [
    'requests>=2.28',
    'rich>=13.0',
]
`

// Quote style is detected from existing entries — single-quoted arrays
// stay single-quoted on add.
func TestEditDepArray_PreservesQuoteStyle(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(singleQuotedSample), "project", "dependencies", EditAdd, "click>=8.0", "click")
	assert.NoErr(err)
	assert.True(strings.Contains(string(out), "    'click>=8.0',\n"))
	assert.True(!strings.Contains(string(out), `"click>=8.0"`))
}

const noTrailingCommaSample = `[project]
name = "demo"
dependencies = [
    "requests>=2.28",
    "rich>=13.0"
]
`

// Trailing-comma style is preserved on the FINAL entry only.
// Pre-existing last entry MUST gain a comma when something gets
// inserted after it — otherwise TOML rejects two array entries on
// consecutive lines with no separator. The "no trailing comma"
// affordance only applies to whichever entry ends up last.
func TestEditDepArray_NoTrailingCommaPreservedOnNewLast(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(noTrailingCommaSample), "project", "dependencies", EditAdd, "click>=8.0", "click")
	assert.NoErr(err)
	got := string(out)
	// New entry sits at the end without a trailing comma.
	assert.True(strings.Contains(got, "    \"click>=8.0\"\n]"))
	// Previous last entry now has a comma — without it the TOML is
	// invalid (two consecutive entries with no separator).
	assert.True(strings.Contains(got, "    \"rich>=13.0\",\n"))
	// And the resulting bytes must round-trip through the parser.
	if _, err := ParsePyProject([]byte(got)); err != nil {
		t.Fatalf("post-edit pyproject did not re-parse: %v\n%s", err, got)
	}
}

const singleLineSample = `[project]
name = "demo"
dependencies = ["requests>=2.28", "rich>=13.0"]
`

// Single-line arrays expand to multi-line on first edit so the result
// stays readable. Caller's only complaint about reformatting is that
// it changed siblings; this is the targeted array, and expansion is
// a deliberate improvement.
func TestEditDepArray_SingleLineExpandsToMultiline(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(singleLineSample), "project", "dependencies", EditAdd, "click>=8.0", "click")
	assert.NoErr(err)
	got := string(out)
	assert.True(strings.Contains(got, "dependencies = [\n"))
	assert.True(strings.Contains(got, "    \"click>=8.0\","))
}

const pep735Sample = `[dependency-groups]
dev = [
    "pytest>=8.0",
]
test = [
    "coverage>=7.0",
]
`

// PEP 735 [dependency-groups] is structurally the same as
// [project].dependencies — an array under a named key. The editor
// targets a specific group via the key parameter.
func TestEditDepArray_PEP735Group(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(pep735Sample), "dependency-groups", "dev", EditAdd, "ruff>=0.1", "ruff")
	assert.NoErr(err)

	got := string(out)
	assert.True(strings.Contains(got, "    \"ruff>=0.1\","))
	// The other group must be untouched.
	assert.True(strings.Contains(got, "test = [\n    \"coverage>=7.0\",\n]"))
}

const emptyArraySample = `[project]
name = "demo"
dependencies = []
`

func TestEditDepArray_EmptyArrayInitialAdd(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(emptyArraySample), "project", "dependencies", EditAdd, "requests>=2.28", "requests")
	assert.NoErr(err)
	got := string(out)
	assert.True(strings.Contains(got, "    \"requests>=2.28\","))
}

const noArrayKeySample = `[project]
name = "demo"
version = "0.1.0"
`

// When the array doesn't exist yet, add creates it; remove is a no-op.
func TestEditDepArray_MissingKeyAddCreates(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(noArrayKeySample), "project", "dependencies", EditAdd, "requests>=2.28", "requests")
	assert.NoErr(err)
	got := string(out)
	assert.True(strings.Contains(got, "dependencies = [\n    \"requests>=2.28\",\n]"))
}

func TestEditDepArray_MissingKeyRemoveNoOp(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(noArrayKeySample), "project", "dependencies", EditRemove, "", "requests")
	assert.NoErr(err)
	assert.Equal(string(out), noArrayKeySample)
}

const extrasSample = `[project]
name = "demo"
dependencies = [
    "fastapi[standard]>=0.115.6",
    "pydantic[email]>=2.9.2",
]
`

// Names with extras (`fastapi[standard]`) match by their bare name.
func TestEditDepArray_RemoveByNameWithExtras(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(extrasSample), "project", "dependencies", EditRemove, "", "fastapi")
	assert.NoErr(err)
	got := string(out)
	assert.True(!strings.Contains(got, "fastapi"))
	assert.True(strings.Contains(got, `"pydantic[email]>=2.9.2"`))
}

const commentedSample = `[project]
name = "demo"
dependencies = [
    # pinned for CVE-2024-XXXX
    "requests>=2.28",
    "rich>=13.0",
]
`

// A standalone comment line preceding a dep entry must survive a
// remove of the entry it precedes — comments belong to the array,
// not the line they happen to sit above.
func TestEditDepArray_PreservesPrecedingComment(t *testing.T) {
	assert := is.New(t)

	out, err := EditDepArray([]byte(commentedSample), "project", "dependencies", EditRemove, "", "requests")
	assert.NoErr(err)
	got := string(out)
	assert.True(strings.Contains(got, "# pinned for CVE-2024-XXXX"))
	assert.True(!strings.Contains(got, "requests>=2.28"))
	assert.True(strings.Contains(got, `"rich>=13.0"`))
}
