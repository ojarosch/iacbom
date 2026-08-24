package scan

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// gitFiles returns the tracked file list via `git ls-files` when available.
// It never modifies git state and degrades gracefully (ok=false) when git
// is unavailable or the root is not a work tree.
func gitFiles(root string) ([]string, bool) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var files []string
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" {
			continue
		}
		files = append(files, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return files, true
}

// GitInfo collects local repository metadata. All fields are informational.
type GitInfo struct {
	IsGit  bool
	Commit string
	Branch string
	Dirty  bool
}

func GitMetadata(root string) GitInfo {
	var info GitInfo
	if !hasGit(root) {
		return info
	}
	info.IsGit = true
	info.Commit, _ = gitOut(root, "rev-parse", "HEAD")
	info.Branch, _ = gitOut(root, "rev-parse", "--abbrev-ref", "HEAD")
	status, _ := gitOut(root, "status", "--porcelain")
	info.Dirty = strings.TrimSpace(status) != ""
	return info
}

func hasGit(root string) bool {
	return exec.Command("git", "-C", root, "rev-parse", "--git-dir").Run() == nil
}

func gitOut(root string, args ...string) (string, bool) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
