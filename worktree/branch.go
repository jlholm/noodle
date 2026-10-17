package worktree

import (
	"fmt"
	"strings"
)

// CurrentBranch returns the integration branch this app merges into.
func (a *App) CurrentBranch() (string, error) {
	branch := strings.TrimSpace(a.integrationBranch())
	if branch == "" {
		return "", fmt.Errorf("integration branch unavailable")
	}
	return branch, nil
}

// CheckedOutBranch returns the branch checked out at path.
// An empty branch with a nil error means the checkout is detached.
func (a *App) CheckedOutBranch(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("worktree path missing")
	}
	out, err := a.gitOutput("-C", path, "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("branch at %s unavailable: %w", path, err)
	}
	return strings.TrimSpace(out), nil
}

// BranchExists reports whether a local branch ref exists.
func (a *App) BranchExists(branch string) bool {
	return a.branchExists(branch)
}

// BranchWorktree returns the checkout path of a local branch.
// attached is false when the branch exists nowhere in the worktree list.
func (a *App) BranchWorktree(branch string) (string, bool, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", false, fmt.Errorf("branch name missing")
	}
	out, err := a.gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("list worktrees: %w", err)
	}
	want := "refs/heads/" + branch
	var path string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			if ref == want {
				return path, true, nil
			}
		}
	}
	return "", false, nil
}

// CreateBranch adds a worktree named name on a new branch starting at base.
func (a *App) CreateBranch(name, branch, base string) error {
	name = strings.TrimSpace(name)
	branch = strings.TrimSpace(branch)
	base = strings.TrimSpace(base)
	if name == "" {
		return fmt.Errorf("worktree name missing")
	}
	if branch == "" {
		return fmt.Errorf("branch name missing")
	}
	if base == "" {
		return fmt.Errorf("base branch missing")
	}
	wtPath := WorktreePath(a.Root, name)
	a.ensureWorktreesGitignored()
	a.info(fmt.Sprintf("Creating worktree at %s...", wtPath))
	if err := a.gitRunWithDetails("worktree", "add", "-b", branch, wtPath, base); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}
	a.finishWorktree(wtPath, branch)
	return nil
}

// AttachBranch adds a worktree named name checking out an existing branch.
func (a *App) AttachBranch(name, branch string) error {
	name = strings.TrimSpace(name)
	branch = strings.TrimSpace(branch)
	if name == "" {
		return fmt.Errorf("worktree name missing")
	}
	if branch == "" {
		return fmt.Errorf("branch name missing")
	}
	wtPath := WorktreePath(a.Root, name)
	a.ensureWorktreesGitignored()
	a.info(fmt.Sprintf("Attaching worktree at %s...", wtPath))
	if err := a.gitRunWithDetails("worktree", "add", wtPath, branch); err != nil {
		return fmt.Errorf("failed to attach worktree: %w", err)
	}
	a.finishWorktree(wtPath, branch)
	return nil
}

// Clean reports whether path has no staged, unstaged, or untracked changes.
func (a *App) Clean(path string) (bool, error) {
	return a.isWorktreeClean(path)
}

// CommitsAhead reports whether branch has commits cherry does not see on base.
func (a *App) CommitsAhead(branch, base string) (bool, error) {
	branch = strings.TrimSpace(branch)
	base = strings.TrimSpace(base)
	if branch == "" {
		return false, fmt.Errorf("branch name missing")
	}
	if base == "" {
		return false, fmt.Errorf("base branch missing")
	}
	out, err := a.gitOutput("cherry", base, branch)
	if err != nil {
		return false, fmt.Errorf("commits ahead of %s: %w", base, err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return false, nil
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "+") {
			return true, nil
		}
	}
	return false, nil
}
