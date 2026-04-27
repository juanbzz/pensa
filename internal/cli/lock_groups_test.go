package cli

import (
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/pkg/pep508"
)

// filterGroups powers `pensa lock --only` / `--without`. The contract
// is: main always stays; only/without scope the rest of the groups.
// One unsolvable group (e.g. infrastructure with a Windows-only
// transitive on a Linux dev machine) shouldn't gate locking the
// happy-path runtime deps.

func gd(name, group string) pyproject.GroupedDependency {
	return pyproject.GroupedDependency{
		Dep:   pep508.Dependency{Name: name},
		Group: group,
	}
}

func names(deps []pyproject.GroupedDependency) []string {
	out := make([]string, len(deps))
	for i, d := range deps {
		out[i] = d.Dep.Name
	}
	return out
}

func TestFilterGroups_NoFlagsKeepsEverything(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
		gd("pulumi", "infrastructure"),
	}
	out := filterGroups(deps, nil, nil)
	assert.Equal(names(out), []string{"django", "black", "pulumi"})
}

func TestFilterGroups_WithoutSkipsNamedGroup(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
		gd("pulumi", "infrastructure"),
	}
	out := filterGroups(deps, nil, []string{"infrastructure"})
	assert.Equal(names(out), []string{"django", "black"})
}

func TestFilterGroups_OnlyKeepsNamedGroupAndMain(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
		gd("pulumi", "infrastructure"),
		gd("pulumi-tls", "bootstrap"),
	}
	// --only infrastructure → main + infrastructure (NOT dev or bootstrap)
	out := filterGroups(deps, []string{"infrastructure"}, nil)
	assert.Equal(names(out), []string{"django", "pulumi"})
}

func TestFilterGroups_MainAlwaysRetained(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
	}
	// Even when main isn't named in --only, it stays.
	out := filterGroups(deps, []string{"dev"}, nil)
	assert.Equal(names(out), []string{"django", "black"})

	// Same when --without names main — the request is ignored
	// (locking without main makes no sense); main stays, and the
	// other groups remain unaffected.
	out = filterGroups(deps, nil, []string{"main"})
	assert.Equal(names(out), []string{"django", "black"})
}

func TestFilterGroups_OnlyWithMultipleGroups(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
		gd("pulumi", "infrastructure"),
		gd("pulumi-tls", "bootstrap"),
	}
	out := filterGroups(deps, []string{"infrastructure", "bootstrap"}, nil)
	assert.Equal(names(out), []string{"django", "pulumi", "pulumi-tls"})
}

// excludedGroupsFor records what got dropped so subsequent
// `pensa add` / `pensa remove` runs can re-apply the same scope.
// Both the `--without` shape and the `--only` shape (which is
// inverted before storage) must produce a stable, sorted list.

func TestExcludedGroupsFor_NoFlagsNil(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
	}
	assert.Equal(excludedGroupsFor(deps, nil, nil), []string(nil))
}

func TestExcludedGroupsFor_WithoutCopiesAndSorts(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
	}
	got := excludedGroupsFor(deps, nil, []string{"infra", "bootstrap"})
	assert.Equal(got, []string{"bootstrap", "infra"})
}

func TestExcludedGroupsFor_OnlyInvertsToExcludedSet(t *testing.T) {
	assert := is.New(t)
	deps := []pyproject.GroupedDependency{
		gd("django", "main"),
		gd("black", "dev"),
		gd("pulumi", "infrastructure"),
		gd("pulumi-tls", "bootstrap"),
	}
	// --only infrastructure → main + infrastructure are kept; the
	// excluded set is dev + bootstrap (sorted).
	got := excludedGroupsFor(deps, []string{"infrastructure"}, nil)
	assert.Equal(got, []string{"bootstrap", "dev"})
}
