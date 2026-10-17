package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateBranchUsesDistinctWorktreeName(t *testing.T) {
	skipWorktreeIntegrationShort(t)
	dir := setupTestRepo(t)
	app := &App{Root: dir, IntegrationBranch: "main", Quiet: true}

	if err := app.CreateBranch("order-42", "noodle/42-fix-login", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	wtPath := WorktreePath(dir, "order-42")
	branch, err := app.CheckedOutBranch(wtPath)
	if err != nil {
		t.Fatalf("CheckedOutBranch: %v", err)
	}
	if branch != "noodle/42-fix-login" {
		t.Fatalf("checked out branch = %q", branch)
	}
	attached, ok, err := app.BranchWorktree("noodle/42-fix-login")
	if err != nil {
		t.Fatalf("BranchWorktree: %v", err)
	}
	if !ok || canonicalPath(attached) != canonicalPath(wtPath) {
		t.Fatalf("attached = %q ok=%v, want %q", attached, ok, wtPath)
	}
	clean, err := app.Clean(wtPath)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !clean {
		t.Fatal("new worktree should be clean")
	}
	ahead, err := app.CommitsAhead("noodle/42-fix-login", "main")
	if err != nil {
		t.Fatalf("CommitsAhead: %v", err)
	}
	if ahead {
		t.Fatal("new branch should not be ahead of main")
	}
}

func TestAttachBranchReusesUnattachedBranch(t *testing.T) {
	skipWorktreeIntegrationShort(t)
	dir := setupTestRepo(t)
	app := &App{Root: dir, IntegrationBranch: "main", Quiet: true}
	if err := app.CreateBranch("order-42", "noodle/42-fix-login", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	wtPath := WorktreePath(dir, "order-42")
	runGitIn(t, dir, "worktree", "remove", wtPath)
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("removed worktree still present: %v", err)
	}
	if !app.BranchExists("noodle/42-fix-login") {
		t.Fatal("branch missing after worktree remove")
	}
	attached, ok, err := app.BranchWorktree("noodle/42-fix-login")
	if err != nil {
		t.Fatalf("BranchWorktree: %v", err)
	}
	if ok {
		t.Fatalf("branch still attached at %s", attached)
	}

	if err := app.AttachBranch("order-42", "noodle/42-fix-login"); err != nil {
		t.Fatalf("AttachBranch: %v", err)
	}
	branch, err := app.CheckedOutBranch(wtPath)
	if err != nil {
		t.Fatalf("CheckedOutBranch: %v", err)
	}
	if branch != "noodle/42-fix-login" {
		t.Fatalf("checked out branch = %q", branch)
	}
}

func TestCleanAndCommitsAheadSeeWorktreeState(t *testing.T) {
	skipWorktreeIntegrationShort(t)
	dir := setupTestRepo(t)
	app := &App{Root: dir, IntegrationBranch: "main", Quiet: true}
	if err := app.CreateBranch("order-7", "noodle/7-item", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	wtPath := WorktreePath(dir, "order-7")
	if err := os.WriteFile(filepath.Join(wtPath, "note.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean, err := app.Clean(wtPath)
	if err != nil {
		t.Fatalf("Clean dirty: %v", err)
	}
	if clean {
		t.Fatal("untracked file should leave the worktree dirty")
	}

	runGitIn(t, wtPath, "add", "note.txt")
	runGitIn(t, wtPath, "commit", "-m", "keep the note")
	clean, err = app.Clean(wtPath)
	if err != nil {
		t.Fatalf("Clean committed: %v", err)
	}
	if !clean {
		t.Fatal("committed worktree should be clean")
	}
	ahead, err := app.CommitsAhead("noodle/7-item", "main")
	if err != nil {
		t.Fatalf("CommitsAhead: %v", err)
	}
	if !ahead {
		t.Fatal("committed branch should be ahead of main")
	}

	current, err := app.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if current != "main" {
		t.Fatalf("CurrentBranch = %q", current)
	}
}
