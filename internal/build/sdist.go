package build

import "github.com/juanbzz/pensa/internal/python"

func BuildFromSdist(opts SdistBuildOptions) (string, error)

type SdistBuildOptions struct {
	Name      string // package name
	Version   string // package version
	SdistPath string // path to sdist archive
	OutputDir string // directory to write built wheel to
	Python    *python.PythonInfo
}
