package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffWorktree(t *testing.T) {
	// Set up a temporary git repo with a main branch and a feature branch.
	// Use os.MkdirTemp instead of t.TempDir() because git background
	// processes on macOS can race with TempDir cleanup, failing the test.
	dir, err := os.MkdirTemp("", "TestDiffWorktree-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Initialize repo with an initial commit on main.
	run("init", "-b", "main")
	run("config", "gc.auto", "0")
	run("config", "core.fsmonitor", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial commit")

	// Create a feature branch and add a change.
	run("checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "new.txt")
	run("commit", "-m", "add new.txt")

	// DiffWorktree will fall back to "main" since there's no origin/HEAD.
	result, err := DiffWorktree(dir)
	if err != nil {
		t.Fatalf("DiffWorktree: %v", err)
	}

	if !strings.Contains(result.Diff, "new.txt") {
		t.Errorf("diff should mention new.txt, got:\n%s", result.Diff)
	}
	if !strings.Contains(result.Diff, "+new file") {
		t.Errorf("diff should contain added line, got:\n%s", result.Diff)
	}
	if !strings.Contains(result.Stat, "new.txt") {
		t.Errorf("stat should mention new.txt, got:\n%s", result.Stat)
	}
}

func TestDiffWorktree_NonexistentPath(t *testing.T) {
	_, err := DiffWorktree("/nonexistent/path/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestDiffWorktreeAgainstFeatureBranch(t *testing.T) {
	skipWorktreeIntegrationShort(t)

	dir := setupTestRepo(t)
	runGitIn(t, dir, "checkout", "-b", "feature/pilot")
	writeFile(t, filepath.Join(dir, "pilot.txt"), "integration branch content")
	runGitIn(t, dir, "add", "pilot.txt")
	runGitIn(t, dir, "commit", "-m", "add pilot content")

	runGitIn(t, dir, "checkout", "-b", "noodle/task")
	writeFile(t, filepath.Join(dir, "task.txt"), "task content")
	runGitIn(t, dir, "add", "task.txt")
	runGitIn(t, dir, "commit", "-m", "add task content")

	result, err := DiffWorktreeAgainst(dir, "feature/pilot")
	if err != nil {
		t.Fatalf("DiffWorktreeAgainst: %v", err)
	}
	if !strings.Contains(result.Diff, "task.txt") {
		t.Fatalf("diff should include task.txt, got:\n%s", result.Diff)
	}
	if strings.Contains(result.Diff, "pilot.txt") {
		t.Fatalf("diff should exclude integration branch changes, got:\n%s", result.Diff)
	}
}

func TestDiffWorktreeAgainstIncludesUncommittedChanges(t *testing.T) {
	skipWorktreeIntegrationShort(t)

	dir := setupTestRepo(t)
	runGitIn(t, dir, "checkout", "-b", "noodle/task")
	writeFile(t, filepath.Join(dir, "committed.txt"), "committed content")
	runGitIn(t, dir, "add", "committed.txt")
	runGitIn(t, dir, "commit", "-m", "committed work")
	writeFile(t, filepath.Join(dir, "committed.txt"), "unstaged content")
	writeFile(t, filepath.Join(dir, "staged.txt"), "staged content")
	runGitIn(t, dir, "add", "staged.txt")
	writeFile(t, filepath.Join(dir, "untracked.txt"), "untracked content")
	before, err := gitOutput(dir, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}

	result, err := DiffWorktreeAgainst(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	after, err := gitOutput(dir, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("review changed the working tree status: before %q, after %q", before, after)
	}
	for _, want := range []string{"unstaged content", "staged content", "untracked content"} {
		if !strings.Contains(result.Diff, want) {
			t.Errorf("diff missing %q: %s", want, result.Diff)
		}
	}
	if !result.Uncommitted {
		t.Fatal("diff should report uncommitted changes")
	}
	for _, want := range []string{"committed.txt", "staged.txt", "untracked.txt"} {
		if !strings.Contains(result.Stat, want) {
			t.Errorf("stat missing %q: %s", want, result.Stat)
		}
	}
}
