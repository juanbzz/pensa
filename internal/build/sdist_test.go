package build

import (
	"testing"

	"github.com/matryer/is"
)

// goetry-c7c: setuptools.build_meta's build_wheel invokes bdist_wheel
// from the wheel package, which many sdists don't list explicitly in
// build-system.requires. effectiveBuildRequires adds it ourselves when
// the backend is setuptools — pip/build/uv all do the same.
func TestEffectiveBuildRequires(t *testing.T) {
	cases := []struct {
		name      string
		declared  []string
		backend   string
		wantWheel bool
	}{
		{"setuptools missing wheel — augment", []string{"setuptools>=40.8.0"}, "setuptools.build_meta", true},
		{"setuptools legacy missing wheel — augment", []string{"setuptools"}, "setuptools.build_meta:__legacy__", true},
		{"setuptools already lists wheel — leave alone", []string{"setuptools>=64", "wheel"}, "setuptools.build_meta", true},
		{"setuptools lists wheel with version — leave alone", []string{"setuptools", "wheel>=0.40"}, "setuptools.build_meta", true},
		{"setuptools lists Wheel — case-insensitive", []string{"setuptools", "Wheel"}, "setuptools.build_meta", true},
		{"setuptools lists wheel with extras — leave alone", []string{"setuptools", "wheel[extra]>=0.40"}, "setuptools.build_meta", true},
		{"hatchling — untouched", []string{"hatchling"}, "hatchling.build", false},
		{"flit — untouched", []string{"flit_core>=3.2"}, "flit_core.buildapi", false},
		{"poetry-core — untouched", []string{"poetry-core>=1.0"}, "poetry.core.masonry.api", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := is.New(t)
			got := effectiveBuildRequires(c.declared, c.backend)
			assert.Equal(requiresHas(got, "wheel"), c.wantWheel)
		})
	}
}

// requiresHas needs to recognize the leading PEP 508 distribution name
// regardless of what follows it. Cover the shapes pensa actually sees
// from real pyprojects.
func TestRequiresHas(t *testing.T) {
	assert := is.New(t)

	assert.True(requiresHas([]string{"wheel"}, "wheel"))
	assert.True(requiresHas([]string{"wheel>=0.40"}, "wheel"))
	assert.True(requiresHas([]string{"wheel<1.0,>=0.40"}, "wheel"))
	assert.True(requiresHas([]string{"wheel[extra]"}, "wheel"))
	assert.True(requiresHas([]string{"wheel ; python_version >= '3.10'"}, "wheel"))
	assert.True(requiresHas([]string{"WHEEL"}, "wheel"))            // case-insensitive
	assert.True(requiresHas([]string{"  wheel  "}, "wheel"))        // surrounding whitespace
	assert.True(requiresHas([]string{"setuptools", "wheel"}, "wheel"))

	assert.True(!requiresHas([]string{"setuptools"}, "wheel"))
	assert.True(!requiresHas([]string{"pywheel"}, "wheel")) // must not prefix-match
	assert.True(!requiresHas(nil, "wheel"))
}
