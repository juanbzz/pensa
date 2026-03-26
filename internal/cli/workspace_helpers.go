package cli

import (
	"fmt"

	"github.com/juanbzz/pensa/internal/workspace"
)

// resolveTargetMember determines which workspace member to target.
// Returns nil member (no error) when not in a workspace (single project mode).
func resolveTargetMember(ws *workspace.Workspace, pkgFlag, cwd string) (*workspace.Member, error) {
	if ws == nil {
		return nil, nil
	}

	// Explicit --package flag.
	if pkgFlag != "" {
		m := ws.FindMember(pkgFlag)
		if m == nil {
			return nil, fmt.Errorf("workspace member %q not found\nAvailable: %s", pkgFlag, ws.MemberNames())
		}
		return m, nil
	}

	// Cwd-based detection: if inside a member directory, target that member.
	if m := ws.MemberForDir(cwd); m != nil {
		return m, nil
	}

	return nil, fmt.Errorf("in a workspace root — use --package to specify which member\nAvailable: %s", ws.MemberNames())
}
