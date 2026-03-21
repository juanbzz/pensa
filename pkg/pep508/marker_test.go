package pep508

import "testing"

var testEnv = Environment{
	PythonVersion:                "3.11",
	PythonFullVersion:            "3.11.4",
	OSName:                       "posix",
	SysPlatform:                  "linux",
	PlatformRelease:              "5.15.0",
	PlatformSystem:               "Linux",
	PlatformVersion:              "#1 SMP",
	PlatformMachine:              "x86_64",
	PlatformPythonImplementation: "CPython",
	ImplementationName:           "cpython",
	ImplementationVersion:        "3.11.4",
}

func TestMarker_Evaluate(t *testing.T) {
	tests := []struct {
		marker string
		want   bool
	}{
		{`python_version >= "3.8"`, true},
		{`python_version < "3.0"`, false},
		{`python_version == "3.11"`, true},
		{`python_version != "3.11"`, false},
		{`sys_platform == "linux"`, true},
		{`sys_platform == "win32"`, false},
		{`os_name == "posix"`, true},
		{`platform_machine == "x86_64"`, true},
		{`implementation_name == "cpython"`, true},
		// AND
		{`os_name == "posix" and python_version >= "3.8"`, true},
		{`os_name == "posix" and python_version < "3.0"`, false},
		// OR
		{`python_version >= "3.8" or sys_platform == "win32"`, true},
		{`python_version < "3.0" or sys_platform == "win32"`, false},
		// Parentheses
		{`os_name == "posix" and (python_version >= "3.8" or sys_platform == "win32")`, true},
		// in operator
		{`"linux" in sys_platform`, true},
		{`"win" in sys_platform`, false},
		// not in
		{`"win" not in sys_platform`, true},
		{`"linux" not in sys_platform`, false},
		// Value on left side
		{`"3.11" == python_version`, true},
		// Legacy aliases
		{`os.name == "posix"`, true},
		{`sys.platform == "linux"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.marker, func(t *testing.T) {
			m, err := ParseMarker(tt.marker)
			if err != nil {
				t.Fatalf("ParseMarker(%q) error: %v", tt.marker, err)
			}
			if got := m.Evaluate(testEnv); got != tt.want {
				t.Errorf("marker %q evaluated to %v, want %v", tt.marker, got, tt.want)
			}
		})
	}
}

func TestParseMarker_Empty(t *testing.T) {
	m, err := ParseMarker("")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Evaluate(testEnv) {
		t.Error("empty marker should evaluate to true")
	}
}

func TestParseMarker_Invalid(t *testing.T) {
	invalids := []string{
		"not_a_marker",
		"python_version",
		`python_version >>>= "3.0"`,
	}
	for _, s := range invalids {
		_, err := ParseMarker(s)
		if err == nil {
			t.Errorf("ParseMarker(%q) expected error", s)
		}
	}
}

func TestMarker_String(t *testing.T) {
	m, err := ParseMarker(`python_version >= "3.8"`)
	if err != nil {
		t.Fatal(err)
	}
	s := m.String()
	if s == "" {
		t.Error("expected non-empty string representation")
	}
}
