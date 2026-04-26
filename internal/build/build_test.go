package build

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

// stripPythonTraceback's job is to drop the CPython "Traceback"
// frames so the user sees the exception line plus any backend
// guidance, not 30 lines of importlib internals.

func TestStripPythonTraceback_DropsFrames(t *testing.T) {
	assert := is.New(t)
	input := strings.Join([]string{
		"Traceback (most recent call last):",
		`  File "<string>", line 5, in <module>`,
		`  File "/site-packages/hatchling/build.py", line 83, in build_editable`,
		`    return os.path.basename(...)`,
		`  File "/site-packages/hatchling/builders/wheel.py", line 536, in foo`,
		`    for included_file in self.recurse():`,
		"ValueError: Unable to determine which files to ship",
		"",
		"Please add the following config to your pyproject.toml:",
		"",
		`[tool.hatch.build.targets.wheel]`,
		`packages = ["src/foo"]`,
	}, "\n")

	got := stripPythonTraceback(input)
	assert.True(!strings.Contains(got, "Traceback (most recent call last):"))
	assert.True(!strings.Contains(got, `File "<string>"`))
	assert.True(strings.Contains(got, "ValueError: Unable to determine"))
	assert.True(strings.Contains(got, `packages = ["src/foo"]`))
}

func TestStripPythonTraceback_NoMarkerReturnsInput(t *testing.T) {
	assert := is.New(t)
	input := "ValueError: missing field\nfix it"
	assert.Equal(stripPythonTraceback(input), input)
}

func TestStripPythonTraceback_EmptyInput(t *testing.T) {
	assert := is.New(t)
	assert.Equal(stripPythonTraceback(""), "")
}

func TestLastLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"single line", "single line"},
		{"line1\nline2\nline3", "line3"},
		{"Traceback:\n  File foo\nAttributeError: bad", "AttributeError: bad"},
		{"\n\n", ""},
	}
	for _, tt := range tests {
		got := lastLine(tt.input)
		if got != tt.want {
			t.Errorf("lastLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseBackend(t *testing.T) {
	tests := []struct {
		input  string
		module string
		object string
	}{
		{"hatchling.build", "hatchling.build", ""},
		{"poetry.core.masonry.api", "poetry.core.masonry.api", ""},
		{"setuptools.build_meta:__legacy__", "setuptools.build_meta", "__legacy__"},
		{"my_backend:Builder", "my_backend", "Builder"},
	}

	for _, tt := range tests {
		mod, obj := parseBackend(tt.input)
		if mod != tt.module || obj != tt.object {
			t.Errorf("parseBackend(%q) = (%q, %q), want (%q, %q)",
				tt.input, mod, obj, tt.module, tt.object)
		}
	}
}
