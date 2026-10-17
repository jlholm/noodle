package loop

import (
	"testing"

	"github.com/poteto/noodle/config"
)

func TestRenderBranchPattern(t *testing.T) {
	got := renderBranchPattern(config.DefaultBranchPattern, "ABC 12", "Fix the bug!")
	if got != "noodle/abc-12-fix-the-bug" {
		t.Fatalf("branch = %q", got)
	}
	worktree := orderWorktreeName("ABC 12")
	if worktree != "abc-12" {
		t.Fatalf("worktree = %q", worktree)
	}
	if worktree == got {
		t.Fatal("worktree name and branch name should differ")
	}

	fallback := renderBranchPattern(config.DefaultBranchPattern, "42", "***")
	if fallback != "noodle/42-item" {
		t.Fatalf("fallback branch = %q", fallback)
	}
	if renderBranchPattern("", "42", "") != "noodle/42-item" {
		t.Fatalf("empty pattern branch = %q", renderBranchPattern("", "42", ""))
	}
	custom := renderBranchPattern("work/{todo_id}", "Todo:9", "ignored title")
	if custom != "work/todo-9" {
		t.Fatalf("custom branch = %q", custom)
	}
}
