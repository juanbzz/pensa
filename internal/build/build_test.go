package build

import "testing"

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
