package cli

import (
	"fmt"
	"io"

	"pensa.sh/pensa/internal/python"
)

// pickPython returns the Python interpreter to use for venv-level operations.
// If a venv already exists at venvPath, its pyvenv.cfg is the source of truth —
// host PATH may point at a different interpreter entirely. If no venv exists,
// discover one from PATH and create it.
func pickPython(w io.Writer, venvPath string) (*python.PythonInfo, error) {
	if python.VenvExists(venvPath) {
		py, err := python.FromVenv(venvPath)
		if err != nil {
			return nil, fmt.Errorf("read venv Python: %w", err)
		}
		fmt.Fprintf(w, "%s %s from %s\n", blue("Using Python"), bold(py.Version), ".venv")
		return py, nil
	}

	py, err := python.Discover()
	if err != nil {
		return nil, fmt.Errorf("find Python: %w", err)
	}
	fmt.Fprintf(w, "%s using Python %s\n", blue("Creating virtualenv"), bold(py.Version))
	if err := python.CreateVenv(venvPath, py); err != nil {
		return nil, fmt.Errorf("create venv: %w", err)
	}
	return py, nil
}
