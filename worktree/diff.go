package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DiffResult holds the unified diff and diffstat for a worktree branch.
type DiffResult struct {
	Diff        string `json:"diff"`
	Stat        string `json:"stat"`
	Uncommitted bool   `json:"uncommitted"`
}

// DiffWorktree compares the worktree's current files with the default base
// branch (auto-discovered from origin/HEAD, falling back to "main").
func DiffWorktree(worktreePath string) (DiffResult, error) {
	return DiffWorktreeAgainst(worktreePath, "")
}

// DiffWorktreeAgainst computes the diff from the base fork to the worktree's
// current files, including staged and untracked changes. An empty base uses
// default-branch discovery.
func DiffWorktreeAgainst(worktreePath, base string) (DiffResult, error) {
	if _, err := os.Stat(worktreePath); err != nil {
		return DiffResult{}, fmt.Errorf("worktree path not found: %s", worktreePath)
	}

	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return DiffResult{}, fmt.Errorf("resolve absolute path: %w", err)
	}

	base = strings.TrimSpace(base)
	if base == "" {
		base = discoverBaseBranch(absPath)
	}
	if strings.HasPrefix(base, "-") {
		return DiffResult{}, fmt.Errorf("invalid base branch %q", base)
	}

	mergeBase, err := gitOutput(absPath, "merge-base", base, "HEAD")
	if err != nil {
		return DiffResult{}, fmt.Errorf("find common ancestor with %s: %w", base, err)
	}
	status, err := gitOutput(absPath, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return DiffResult{}, fmt.Errorf("worktree status: %w", err)
	}

	indexDir, err := os.MkdirTemp("", "noodle-review-index-*")
	if err != nil {
		return DiffResult{}, fmt.Errorf("create review index: %w", err)
	}
	defer os.RemoveAll(indexDir)
	indexPath := filepath.Join(indexDir, "index")

	if _, err := gitOutputWithIndex(absPath, indexPath, "read-tree", "HEAD"); err != nil {
		return DiffResult{}, fmt.Errorf("populate review index: %w", err)
	}
	if _, err := gitOutputWithIndex(absPath, indexPath, "add", "-A"); err != nil {
		return DiffResult{}, fmt.Errorf("stage review files: %w", err)
	}
	diff, err := gitOutputWithIndex(absPath, indexPath, "diff", "--cached", "--no-ext-diff", "--no-textconv", mergeBase)
	if err != nil {
		return DiffResult{}, fmt.Errorf("git diff: %w", err)
	}

	stat, err := gitOutputWithIndex(absPath, indexPath, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--stat", mergeBase)
	if err != nil {
		return DiffResult{}, fmt.Errorf("git diff --stat: %w", err)
	}

	return DiffResult{Diff: diff, Stat: stat, Uncommitted: status != ""}, nil
}

func gitOutputWithIndex(repoPath, indexPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// discoverBaseBranch returns the default branch name from origin/HEAD,
// falling back to "main" on any error.
func discoverBaseBranch(repoPath string) string {
	ref, err := gitOutput(repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main"
	}
	// ref looks like "refs/remotes/origin/main" — extract branch name.
	if name := strings.TrimPrefix(ref, "refs/remotes/origin/"); name != ref {
		// Guard against ref names starting with "-" which git would
		// interpret as option flags.
		if strings.HasPrefix(name, "-") {
			return "main"
		}
		return name
	}
	return "main"
}

func gitOutput(repoPath string, args ...string) (string, error) {
	full := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
