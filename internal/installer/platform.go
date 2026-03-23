package installer

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juanbzz/pensa/internal/python"
)

// compatTag represents a single compatible (python, abi, platform) combination.
type compatTag struct {
	Python   string
	ABI      string
	Platform string
}

// PlatformTags holds an ordered list of compatible wheel tags for the current platform.
// Index 0 is the highest priority (most specific match).
type PlatformTags struct {
	tags []compatTag
}

// NewPlatformTags generates all compatible wheel tags for the current platform
// and Python version, ordered by priority (best match first).
func NewPlatformTags(py *python.PythonInfo) *PlatformTags {
	cpVer := fmt.Sprintf("cp%d%d", py.Major, py.Minor)
	pyVer := fmt.Sprintf("py%d%d", py.Major, py.Minor)
	pyMajor := fmt.Sprintf("py%d", py.Major)

	platforms := platformTags()

	var tags []compatTag

	// 1. CPython exact: cp3XX-cp3XX-{platform} (best match for C extensions)
	for _, plat := range platforms {
		tags = append(tags, compatTag{cpVer, cpVer, plat})
	}

	// 2. CPython abi3: cp3XX-abi3-{platform} (stable ABI)
	for _, plat := range platforms {
		tags = append(tags, compatTag{cpVer, "abi3", plat})
	}

	// 3. CPython none: cp3XX-none-{platform}
	for _, plat := range platforms {
		tags = append(tags, compatTag{cpVer, "none", plat})
	}

	// 4. CPython none any: cp3XX-none-any
	tags = append(tags, compatTag{cpVer, "none", "any"})

	// 5. Pure python versioned: py3XX-none-any
	tags = append(tags, compatTag{pyVer, "none", "any"})

	// 6. Pure python major: py3-none-any (lowest priority, always matches)
	tags = append(tags, compatTag{pyMajor, "none", "any"})

	return &PlatformTags{tags: tags}
}

// Score returns the priority score for a wheel filename.
// Lower score = better match. Returns -1 if the wheel is incompatible.
func (pt *PlatformTags) Score(filename string) int {
	wheelPy, wheelABI, wheelPlat := parseWheelTags(filename)
	if wheelPy == nil {
		return -1
	}

	for i, tag := range pt.tags {
		if sliceContainsTag(wheelPy, tag.Python) &&
			sliceContainsTag(wheelABI, tag.ABI) &&
			sliceContainsTag(wheelPlat, tag.Platform) {
			return i
		}
	}
	return -1
}

// Compatible returns true if the wheel filename is compatible with this platform.
func (pt *PlatformTags) Compatible(filename string) bool {
	return pt.Score(filename) >= 0
}

// parseWheelTags extracts python, abi, and platform tags from a wheel filename.
// Wheel format: {name}-{version}-{python}-{abi}-{platform}.whl
// Tags can have multiple values separated by '.' (e.g., "manylinux_2_17_x86_64.manylinux2014_x86_64")
func parseWheelTags(filename string) (pythonTags, abiTags, platformTags []string) {
	// Strip .whl extension.
	name := strings.TrimSuffix(filename, ".whl")
	if name == filename {
		return nil, nil, nil
	}

	// Split from the right — last 3 segments are python-abi-platform.
	// But platform can contain dots (multi-tag), so we split by '-' and
	// take the last 3 hyphen-separated groups.
	parts := strings.Split(name, "-")
	if len(parts) < 3 {
		return nil, nil, nil
	}

	// Last 3 hyphen-separated parts: python, abi, platform.
	// But the name itself can contain hyphens, so count from the end.
	platStr := parts[len(parts)-1]
	abiStr := parts[len(parts)-2]
	pyStr := parts[len(parts)-3]

	pythonTags = strings.Split(pyStr, ".")
	abiTags = strings.Split(abiStr, ".")
	platformTags = strings.Split(platStr, ".")

	return pythonTags, abiTags, platformTags
}

// sliceContainsTag checks if any element in the slice matches the tag.
func sliceContainsTag(slice []string, tag string) bool {
	for _, s := range slice {
		if s == tag {
			return true
		}
	}
	return false
}

// platformTags returns the compatible platform tags for the current OS and architecture.
func platformTags() []string {
	arch := goArchToWheel(runtime.GOARCH)

	switch runtime.GOOS {
	case "darwin":
		return darwinPlatformTags(arch)
	case "linux":
		return linuxPlatformTags(arch)
	case "windows":
		return windowsPlatformTags(arch)
	default:
		return []string{"any"}
	}
}

func darwinPlatformTags(arch string) []string {
	var tags []string

	if arch == "arm64" {
		// ARM Macs require macOS 11+.
		for v := 14; v >= 11; v-- {
			tags = append(tags, fmt.Sprintf("macosx_%d_0_arm64", v))
		}
		for v := 14; v >= 11; v-- {
			tags = append(tags, fmt.Sprintf("macosx_%d_0_universal2", v))
		}
	} else {
		// Intel Macs, 10.9+.
		for v := 14; v >= 10; v-- {
			if v == 10 {
				for minor := 15; minor >= 9; minor-- {
					tags = append(tags, fmt.Sprintf("macosx_10_%d_x86_64", minor))
				}
			} else {
				tags = append(tags, fmt.Sprintf("macosx_%d_0_x86_64", v))
			}
		}
		for v := 14; v >= 10; v-- {
			if v == 10 {
				tags = append(tags, "macosx_10_9_universal2")
			} else {
				tags = append(tags, fmt.Sprintf("macosx_%d_0_universal2", v))
			}
		}
	}

	return tags
}

func linuxPlatformTags(arch string) []string {
	var tags []string

	// manylinux tags from newest to oldest glibc.
	glibcVersions := [][2]int{
		{2, 35}, {2, 34}, {2, 31}, {2, 28}, {2, 27}, {2, 24}, {2, 17}, {2, 12}, {2, 5},
	}
	for _, gv := range glibcVersions {
		tags = append(tags, fmt.Sprintf("manylinux_%d_%d_%s", gv[0], gv[1], arch))
	}

	// Legacy aliases.
	tags = append(tags, "manylinux2014_"+arch) // alias for 2_17
	tags = append(tags, "manylinux2010_"+arch) // alias for 2_12
	tags = append(tags, "manylinux1_"+arch)    // alias for 2_5

	// Generic linux.
	tags = append(tags, "linux_"+arch)

	return tags
}

func windowsPlatformTags(arch string) []string {
	switch arch {
	case "amd64":
		return []string{"win_amd64"}
	case "arm64":
		return []string{"win_arm64"}
	default:
		return []string{"win32"}
	}
}

func goArchToWheel(goarch string) string {
	switch goarch {
	case "amd64":
		if runtime.GOOS == "windows" {
			return "amd64"
		}
		return "x86_64"
	case "arm64":
		if runtime.GOOS == "linux" {
			return "aarch64"
		}
		return "arm64"
	case "386":
		return "i686"
	default:
		return goarch
	}
}
