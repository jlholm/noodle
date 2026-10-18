package loop

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/state"
	"github.com/poteto/noodle/worktree"
)

func TestRunningCookKeepsCheckoutPolicyWhenCompletionChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}

	t.Run("merge cook stays on merge after a switch to branch", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Mode = "auto"
		env := newGitLoop(t, cfg)
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "merge me")
		env.loop.config.Git.Completion = config.GitCompletionBranch
		env.complete("completed")
		env.cycle()

		if !gitLogContains(t, env.dir, "merge me") {
			t.Fatal("merge cook lost its commit after completion changed to branch")
		}
		if _, err := os.Stat(call.WorktreePath); !os.IsNotExist(err) {
			t.Fatalf("merge worktree still present: %v", err)
		}
		if localBranchExists(env.dir, call.Name) {
			t.Fatalf("stage branch %s still exists", call.Name)
		}
		if len(env.adapter.doneCalls) != 1 || env.adapter.doneCalls[0] != "42" {
			t.Fatalf("done = %#v", env.adapter.doneCalls)
		}
	})

	t.Run("workspace cook stays on branch after a switch to merge", func(t *testing.T) {
		env := newGitLoop(t, branchTestConfig("auto"))
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "keep me")
		env.loop.config.Git.Completion = config.GitCompletionMerge
		env.complete("completed")
		env.cycle()

		reviews, err := ReadPendingReview(env.loop.runtimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(reviews) != 1 || reviews[0].ApprovalAction != approvalActionComplete {
			t.Fatalf("reviews = %#v", reviews)
		}
		if gitLogContains(t, env.dir, "keep me") {
			t.Fatal("workspace cook merged onto the integration branch")
		}
		if _, err := os.Stat(call.WorktreePath); err != nil {
			t.Fatalf("workspace removed: %v", err)
		}
		if !localBranchExists(env.dir, renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")) {
			t.Fatal("workspace branch was deleted")
		}
		if len(env.adapter.doneCalls) != 0 {
			t.Fatalf("done = %#v", env.adapter.doneCalls)
		}
	})
}

func TestPendingReviewFollowsCookCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	cfg := config.DefaultConfig()
	cfg.Mode = "supervised"
	env := newGitLoop(t, cfg)
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()
	call := env.lastCall()
	gitCommit(t, call.WorktreePath, "stage commit")

	node := env.loop.canonical.Orders["42"]
	node.Workspace = &state.OrderWorkspace{
		WorktreeName: "not-the-cook",
		BranchName:   "noodle/not-the-cook",
		BaseBranch:   env.base,
	}
	env.loop.canonical.Orders["42"] = node
	env.loop.config.Git.Completion = config.GitCompletionBranch
	env.complete("completed")
	env.cycle()

	reviews, err := ReadPendingReview(env.loop.runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].ApprovalAction != "" || reviews[0].WorktreeName != call.Name {
		t.Fatalf("reviews = %#v", reviews)
	}
	if err := env.loop.controlMerge("42"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !gitLogContains(t, env.dir, "stage commit") {
		t.Fatal("merge review did not land the stage commit")
	}
}

func TestAdoptedSessionFollowsCookCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}

	t.Run("workspace session stays on branch after a switch to merge", func(t *testing.T) {
		env := newGitLoop(t, branchTestConfig("auto"))
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "adopted work")
		env.loop.config.Git.Completion = config.GitCompletionMerge
		env.adopt("42")
		env.cycle()

		reviews, err := ReadPendingReview(env.loop.runtimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(reviews) != 1 || reviews[0].ApprovalAction != approvalActionComplete || reviews[0].WorktreePath != call.WorktreePath {
			t.Fatalf("reviews = %#v", reviews)
		}
		if gitLogContains(t, env.dir, "adopted work") {
			t.Fatal("adopted workspace session merged onto the integration branch")
		}
		if _, err := os.Stat(call.WorktreePath); err != nil {
			t.Fatalf("workspace removed: %v", err)
		}
	})

	t.Run("merge session stays on merge after a switch to branch", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Mode = "auto"
		env := newGitLoop(t, cfg)
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "adopted merge")
		env.loop.config.Git.Completion = config.GitCompletionBranch
		env.adopt("42")
		env.cycle()

		if !gitLogContains(t, env.dir, "adopted merge") {
			t.Fatal("adopted merge session lost its commit")
		}
		if _, err := os.Stat(call.WorktreePath); !os.IsNotExist(err) {
			t.Fatalf("merge worktree still present: %v", err)
		}
	})

	t.Run("merge session ignores an unrelated order workspace", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Mode = "auto"
		env := newGitLoop(t, cfg)
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "stage commit")
		node := env.loop.canonical.Orders["42"]
		node.Workspace = &state.OrderWorkspace{
			WorktreeName: "not-the-cook",
			BranchName:   "noodle/not-the-cook",
			BaseBranch:   env.base,
		}
		env.loop.canonical.Orders["42"] = node
		env.loop.config.Git.Completion = config.GitCompletionBranch
		env.adopt("42")
		env.cycle()

		if !gitLogContains(t, env.dir, "stage commit") {
			t.Fatal("adopted merge session did not land the stage commit")
		}
		if _, err := os.Stat(call.WorktreePath); !os.IsNotExist(err) {
			t.Fatalf("stage worktree still present: %v", err)
		}
	})
}

func TestSpawnKeepsStoredWorkspaceAfterCompletionSwitch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}

	t.Run("branch stage stays on its checkout after a switch to merge", func(t *testing.T) {
		env := newGitLoop(t, branchTestConfig("supervised"))
		env.setOrders(gitOrder("42", "Fix login",
			gitStage("execute", "execute", "process"),
			gitStage("review", "review", "process"),
		))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "stage one")
		env.loop.config.Git.Completion = config.GitCompletionMerge
		env.complete("completed")
		env.cycle()

		next := env.lastCall()
		wantBranch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
		if next.WorktreePath != call.WorktreePath || currentBranch(t, next.WorktreePath) != wantBranch {
			t.Fatalf("stage 1 dispatch = path %q branch %q", next.WorktreePath, currentBranch(t, next.WorktreePath))
		}
		if !gitLogContains(t, call.WorktreePath, "stage one") {
			t.Fatal("stage 1 lost the stage 0 commit")
		}
		if gitLogContains(t, env.dir, "stage one") {
			t.Fatal("stage 1 merged the stage 0 commit onto the integration branch")
		}
		assertCheckoutNames(t, env.dir, call.Name)
	})

	t.Run("branch retry stays on its checkout after a switch to merge", func(t *testing.T) {
		env := newGitLoop(t, branchTestConfig("supervised"))
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "keep the note")
		env.complete("completed")
		env.cycle()
		env.loop.config.Git.Completion = config.GitCompletionMerge
		if err := env.loop.controlRequestChanges("42", "tighten the copy"); err != nil {
			t.Fatal(err)
		}
		if err := env.loop.controlRequeue("42"); err != nil {
			t.Fatal(err)
		}
		env.cycle()

		again := env.lastCall()
		wantBranch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
		if again.WorktreePath != call.WorktreePath || currentBranch(t, again.WorktreePath) != wantBranch {
			t.Fatalf("requeue dispatch = path %q branch %q", again.WorktreePath, currentBranch(t, again.WorktreePath))
		}
		if !strings.Contains(again.Prompt, "resuming work") || !strings.Contains(again.Prompt, "keep the note") {
			t.Fatalf("retry prompt = %s", again.Prompt)
		}
		if !gitLogContains(t, call.WorktreePath, "keep the note") {
			t.Fatal("retry lost the branch commit")
		}
		assertCheckoutNames(t, env.dir, call.Name)
	})

	t.Run("branch steer stays on its checkout after a switch to merge", func(t *testing.T) {
		env := newGitLoop(t, branchTestConfig("auto"))
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "keep me")
		env.loop.config.Git.Completion = config.GitCompletionMerge
		if err := env.loop.steer(call.Name, "hold the branch"); err != nil {
			t.Fatal(err)
		}

		again := env.lastCall()
		wantBranch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
		if again.WorktreePath != call.WorktreePath || currentBranch(t, again.WorktreePath) != wantBranch {
			t.Fatalf("steer dispatch = path %q branch %q", again.WorktreePath, currentBranch(t, again.WorktreePath))
		}
		if !strings.Contains(again.Prompt, "hold the branch") {
			t.Fatalf("steer prompt = %s", again.Prompt)
		}
		if !gitLogContains(t, call.WorktreePath, "keep me") {
			t.Fatal("steer lost the branch commit")
		}
		assertCheckoutNames(t, env.dir, call.Name)
	})

	t.Run("merge steer stays on merge after a switch to branch", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Mode = "auto"
		env := newGitLoop(t, cfg)
		env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
		env.cycle()
		call := env.lastCall()
		gitCommit(t, call.WorktreePath, "merge me")
		env.loop.config.Git.Completion = config.GitCompletionBranch
		env.loop.config.Git.BranchPattern = config.DefaultBranchPattern
		if err := env.loop.steer(call.Name, "stay here"); err != nil {
			t.Fatal(err)
		}

		again := env.lastCall()
		if again.WorktreePath != call.WorktreePath {
			t.Fatalf("steer path = %q, want %q", again.WorktreePath, call.WorktreePath)
		}
		if !strings.Contains(again.Prompt, "stay here") || !gitLogContains(t, call.WorktreePath, "merge me") {
			t.Fatal("merge steer left its checkout")
		}
		if _, ok := env.loop.orderWorkspace("42"); ok {
			t.Fatal("merge steer bound an order workspace")
		}
		wantBranch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
		if localBranchExists(env.dir, wantBranch) {
			t.Fatalf("orphan branch %s exists", wantBranch)
		}
		assertCheckoutNames(t, env.dir, call.Name)
	})
}

func assertCheckoutNames(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			got = append(got, entry.Name())
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("checkouts = %v, want %v", got, want)
	}
}

func TestBranchWorkspaceRecoversDeletedCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	branch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
	name := orderWorktreeName("42")
	if err := env.app.CreateBranch(name, branch, env.base); err != nil {
		t.Fatalf("precreate: %v", err)
	}
	path := worktree.WorktreePath(env.dir, name)
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if n := countBranchCheckouts(t, env.dir, branch); n != 1 {
		t.Fatalf("stale checkout count = %d, want 1", n)
	}

	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()

	call := env.lastCall()
	if call.WorktreePath != path {
		t.Fatalf("dispatch path = %q, want %q", call.WorktreePath, path)
	}
	if got := currentBranch(t, call.WorktreePath); got != branch {
		t.Fatalf("branch = %q, want %q", got, branch)
	}
	if n := countBranchCheckouts(t, env.dir, branch); n != 1 {
		t.Fatalf("branch %s checked out %d times", branch, n)
	}
	orders := env.readOrders()
	if len(orders.Orders) != 1 || orders.Orders[0].Status != OrderStatusActive {
		t.Fatalf("orders = %#v", orders.Orders)
	}
}

func TestBlockingStageMessageOnNonFinalBranchStageContinues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("42", "Fix login",
		gitStage("execute", "execute", "process"),
		gitStage("review", "review", "process"),
	))
	env.cycle()
	call := env.lastCall()
	gitCommit(t, call.WorktreePath, "stage one")
	if err := os.WriteFile(filepath.Join(call.WorktreePath, "dirty.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.emitStageMessage("hold for notes", true)
	env.complete("completed")
	env.cycle()

	reviews, err := ReadPendingReview(env.loop.runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].ApprovalAction != approvalActionContinue {
		t.Fatalf("intermediate reviews = %#v", reviews)
	}
	if len(env.adapter.doneCalls) != 0 {
		t.Fatalf("done after intermediate review = %#v", env.adapter.doneCalls)
	}
	if err := env.loop.controlMerge("42"); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if gitLogContains(t, env.dir, "stage one") {
		t.Fatal("continuing the stage merged it onto the integration branch")
	}
	if _, err := os.Stat(filepath.Join(call.WorktreePath, "dirty.txt")); err != nil {
		t.Fatalf("continue removed the worktree: %v", err)
	}
	orders := env.readOrders()
	if len(orders.Orders) != 1 || orders.Orders[0].Status != OrderStatusActive {
		t.Fatalf("orders after continue = %#v", orders.Orders)
	}
	if orders.Orders[0].Stages[0].Status != StageStatusCompleted {
		t.Fatalf("stage 1 status = %s", orders.Orders[0].Stages[0].Status)
	}

	env.cycle()
	next := env.lastCall()
	if next.WorktreePath != call.WorktreePath {
		t.Fatalf("next stage path = %q, want %q", next.WorktreePath, call.WorktreePath)
	}

	env.emitStageMessage("final hold", true)
	env.complete("completed")
	env.cycle()
	reviews, err = ReadPendingReview(env.loop.runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].ApprovalAction != approvalActionComplete {
		t.Fatalf("final reviews = %#v", reviews)
	}
	if err := env.loop.controlMerge("42"); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("final approval error = %v", err)
	}
	gitCommit(t, call.WorktreePath, "stage two")
	if err := env.loop.controlMerge("42"); err != nil {
		t.Fatalf("final approval: %v", err)
	}
	if len(env.adapter.doneCalls) != 1 || env.adapter.doneCalls[0] != "42" {
		t.Fatalf("done = %#v", env.adapter.doneCalls)
	}
	if gitLogContains(t, env.dir, "stage two") {
		t.Fatal("final approval merged onto the integration branch")
	}
	if _, err := os.Stat(call.WorktreePath); err != nil {
		t.Fatalf("final approval removed the worktree: %v", err)
	}
}

func TestBranchWorkspaceRejectsCollidingOrders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	cfg := branchTestConfig("auto")
	cfg.Git.BranchPattern = "{todo_id}"
	env := newGitLoop(t, cfg)
	env.setOrders(
		gitOrder("Foo", "One", gitStage("execute", "execute", "process")),
		gitOrder("foo", "Two", gitStage("execute", "execute", "process")),
	)
	env.cycle()

	if len(env.rt.calls) != 1 {
		t.Fatalf("dispatches = %d, want 1", len(env.rt.calls))
	}
	orders := env.readOrders()
	var fooActive, lowerFailed bool
	for _, order := range orders.Orders {
		switch order.ID {
		case "Foo":
			fooActive = order.Status == OrderStatusActive
		case "foo":
			lowerFailed = order.Status == OrderStatusFailed
		}
	}
	if !fooActive || !lowerFailed {
		t.Fatalf("orders = %#v", orders.Orders)
	}
	if n := countBranchCheckouts(t, env.dir, "foo"); n != 1 {
		t.Fatalf("branch foo checked out %d times", n)
	}
}

func TestBranchWorkspaceRejectsCollidingBranchName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("auto"))
	env.setOrders(gitOrder("alpha", "One", gitStage("execute", "execute", "process")))
	env.cycle()
	want := renderBranchPattern(config.DefaultBranchPattern, "beta", "Two")
	node := env.loop.canonical.Orders["alpha"]
	if node.Workspace == nil {
		t.Fatal("alpha workspace missing")
	}
	node.Workspace.BranchName = want
	env.loop.canonical.Orders["alpha"] = node

	current := env.readOrders()
	if err := env.loop.writeOrdersState(OrdersFile{Orders: append(current.Orders, gitOrder("beta", "Two", gitStage("execute", "execute", "process")))}); err != nil {
		t.Fatal(err)
	}
	node = env.loop.canonical.Orders["alpha"]
	if node.Workspace == nil {
		t.Fatal("alpha workspace missing after orders update")
	}
	node.Workspace.BranchName = want
	env.loop.canonical.Orders["alpha"] = node
	env.cycle()

	orders := env.readOrders()
	var betaFailed bool
	for _, order := range orders.Orders {
		if order.ID == "beta" && order.Status == OrderStatusFailed {
			betaFailed = true
		}
	}
	if !betaFailed {
		t.Fatalf("orders = %#v", orders.Orders)
	}
	if localBranchExists(env.dir, want) {
		t.Fatalf("created colliding branch %s", want)
	}
	if n := countBranchCheckouts(t, env.dir, renderBranchPattern(config.DefaultBranchPattern, "alpha", "One")); n != 1 {
		t.Fatalf("alpha checkout count = %d", n)
	}
}

func TestBranchWorkspaceRejectsInvalidRenderedRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("auto"))
	env.loop.config.Git.BranchPattern = "bad..{todo_id}"
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()

	orders := env.readOrders()
	if len(orders.Orders) != 1 || orders.Orders[0].Status != OrderStatusFailed {
		t.Fatalf("orders = %#v", orders.Orders)
	}
	if localBranchExists(env.dir, "bad..42") {
		t.Fatal("created invalid branch")
	}
	if _, err := os.Stat(worktree.WorktreePath(env.dir, orderWorktreeName("42"))); !os.IsNotExist(err) {
		t.Fatalf("invalid ref left a worktree: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(env.loop.runtimeDir, "loop-events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "failed to create worktree") {
		t.Fatal("git mutation ran before ref validation")
	}
	if !strings.Contains(text, "branch ref") {
		t.Fatalf("stage failure did not name the branch ref: %s", text)
	}
}
