package loop

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/event"
	loopruntime "github.com/poteto/noodle/runtime"
	"github.com/poteto/noodle/worktree"
)

func TestBranchWorkspaceTwoStagesShareCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("42", "Fix login",
		gitStage("execute", "execute", "process"),
		gitStage("review", "review", "process"),
	))

	env.cycle()
	first := env.lastCall()
	wantBranch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
	wantTree := orderWorktreeName("42")
	if wantTree == wantBranch {
		t.Fatalf("worktree %q and branch %q should differ", wantTree, wantBranch)
	}
	if first.Name != wantTree || first.WorktreePath != filepath.Join(env.dir, ".worktrees", wantTree) {
		t.Fatalf("first dispatch = name %q path %q", first.Name, first.WorktreePath)
	}
	if got := currentBranch(t, first.WorktreePath); got != wantBranch {
		t.Fatalf("branch = %q, want %q", got, wantBranch)
	}

	orders, err := env.loop.currentOrders()
	if err != nil {
		t.Fatal(err)
	}
	orders.Orders[0].Title = "Entirely different title"
	if err := env.loop.writeOrdersState(orders); err != nil {
		t.Fatal(err)
	}
	env.loop.config.Git.BranchPattern = "elsewhere/{slug}"

	gitCommit(t, first.WorktreePath, "stage one")
	note := filepath.Join(first.WorktreePath, "note.txt")
	if err := os.WriteFile(note, []byte("still dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.complete("completed")
	env.cycle()

	if reviews, err := ReadPendingReview(env.loop.runtimeDir); err != nil || len(reviews) != 0 {
		t.Fatalf("intermediate reviews = %#v err=%v", reviews, err)
	}
	if _, err := os.Stat(note); err != nil {
		t.Fatalf("intermediate stage removed %s: %v", note, err)
	}
	second := env.lastCall()
	if second.WorktreePath != first.WorktreePath {
		t.Fatalf("stage 2 path = %q, want %q", second.WorktreePath, first.WorktreePath)
	}
	if strings.Contains(second.Prompt, "resuming work") {
		t.Fatalf("later stage prompt looks like a retry:\n%s", second.Prompt)
	}
	if got := currentBranch(t, second.WorktreePath); got != wantBranch {
		t.Fatalf("branch after title edit = %q, want %q", got, wantBranch)
	}

	gitCommit(t, second.WorktreePath, "stage two")
	env.complete("completed")
	env.cycle()

	reviews, err := ReadPendingReview(env.loop.runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews = %#v", reviews)
	}
	review := reviews[0]
	if review.ApprovalAction != approvalActionComplete || review.BranchName != wantBranch || review.BaseBranch != env.base {
		t.Fatalf("review = %#v", review)
	}
	if review.WorktreePath != first.WorktreePath {
		t.Fatalf("review path = %q", review.WorktreePath)
	}
}

func TestBranchWorkspaceAdoptsMatchingWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	branch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
	name := orderWorktreeName("42")
	if err := env.app.CreateBranch(name, branch, env.base); err != nil {
		t.Fatalf("precreate: %v", err)
	}
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()

	call := env.lastCall()
	if call.WorktreePath != worktree.WorktreePath(env.dir, name) {
		t.Fatalf("dispatch path = %q", call.WorktreePath)
	}
	if got := currentBranch(t, call.WorktreePath); got != branch {
		t.Fatalf("branch = %q", got)
	}
	ws, ok := env.loop.orderWorkspace("42")
	if !ok || ws.BranchName != branch || ws.WorktreeName != name || ws.BaseBranch != env.base {
		t.Fatalf("workspace = %#v ok=%v", ws, ok)
	}
	if countBranchCheckouts(t, env.dir, branch) != 1 {
		t.Fatalf("branch %s checked out more than once", branch)
	}
}

func TestBranchWorkspaceAttachesUnattachedBranch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	branch := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
	name := orderWorktreeName("42")
	git(t, env.dir, "branch", branch)
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()

	call := env.lastCall()
	if call.WorktreePath != worktree.WorktreePath(env.dir, name) {
		t.Fatalf("dispatch path = %q", call.WorktreePath)
	}
	if got := currentBranch(t, call.WorktreePath); got != branch {
		t.Fatalf("branch = %q, want %q", got, branch)
	}
	if n := countBranchCheckouts(t, env.dir, branch); n != 1 {
		t.Fatalf("branch %s checked out %d times", branch, n)
	}
}

func TestBranchWorkspaceRejectsMismatchedWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	name := orderWorktreeName("42")
	if err := env.app.CreateBranch(name, "someone-else", env.base); err != nil {
		t.Fatalf("precreate: %v", err)
	}
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()

	orders := env.readOrders()
	if len(orders.Orders) != 1 || orders.Orders[0].Status != OrderStatusFailed {
		t.Fatalf("orders = %#v", orders.Orders)
	}
	if _, ok := env.loop.orderWorkspace("42"); ok {
		t.Fatal("mismatched worktree was bound")
	}
	path := worktree.WorktreePath(env.dir, name)
	if got := currentBranch(t, path); got != "someone-else" {
		t.Fatalf("branch = %q", got)
	}
	want := renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")
	if localBranchExists(env.dir, want) {
		t.Fatalf("created unexpected branch %s", want)
	}
	if len(env.rt.calls) != 0 {
		t.Fatalf("dispatched despite mismatch: %#v", env.rt.calls)
	}
}

func TestBranchWorkspaceRejectsRemoteRuntimeBeforeCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(
		gitOrder("bad", "Sprites task", gitStage("execute", "execute", "sprites")),
		gitOrder("good", "Local task", gitStage("execute", "execute", "process")),
	)
	env.cycle()

	orders := env.readOrders()
	var badFailed, goodActive bool
	for _, order := range orders.Orders {
		switch order.ID {
		case "bad":
			badFailed = order.Status == OrderStatusFailed
		case "good":
			goodActive = order.Status == OrderStatusActive
		}
	}
	if !badFailed || !goodActive {
		t.Fatalf("orders = %#v", orders.Orders)
	}
	if _, err := os.Stat(worktree.WorktreePath(env.dir, orderWorktreeName("bad"))); !os.IsNotExist(err) {
		t.Fatalf("bad order left a worktree: %v", err)
	}
	if _, ok := env.loop.orderWorkspace("bad"); ok {
		t.Fatal("bad order bound a workspace")
	}
	if _, err := os.Stat(worktree.WorktreePath(env.dir, orderWorktreeName("good"))); err != nil {
		t.Fatalf("good worktree: %v", err)
	}
}

func TestBootstrapOrderStaysOnMergeMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	cfg := branchTestConfig("auto")
	env := newGitLoop(t, cfg)
	order := scheduleBootstrapOopsOrder(cfg)
	env.setOrders(order)
	env.cycle()

	call := env.lastCall()
	stageName := cookBaseName(order.ID, 0, order.Stages[0].TaskKey)
	if call.Name != stageName {
		t.Fatalf("bootstrap worktree = %q, want %q", call.Name, stageName)
	}
	if _, ok := env.loop.orderWorkspace(order.ID); ok {
		t.Fatal("bootstrap bound an order workspace")
	}
	gitCommit(t, call.WorktreePath, "bootstrap skill")
	env.complete("completed")
	env.cycle()

	if _, err := os.Stat(call.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("bootstrap worktree still present: %v", err)
	}
	if !gitLogContains(t, env.dir, "bootstrap skill") {
		t.Fatal("bootstrap commit did not land on the integration branch")
	}
}

func TestMergeModeStillMergesStageWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	cfg := config.DefaultConfig()
	cfg.Mode = "auto"
	env := newGitLoop(t, cfg)
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()

	call := env.lastCall()
	if call.Name != cookBaseName("42", 0, "execute") {
		t.Fatalf("merge worktree = %q", call.Name)
	}
	gitCommit(t, call.WorktreePath, "merge me")
	env.complete("completed")
	env.cycle()

	if _, err := os.Stat(call.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("merge worktree still present: %v", err)
	}
	if localBranchExists(env.dir, call.Name) {
		t.Fatalf("stage branch %s still exists", call.Name)
	}
	if !gitLogContains(t, env.dir, "merge me") {
		t.Fatal("merge commit missing from the integration branch")
	}
	if len(env.adapter.doneCalls) != 1 || env.adapter.doneCalls[0] != "42" {
		t.Fatalf("done = %#v", env.adapter.doneCalls)
	}
}

func TestBranchWorkspaceDirtyApprovalStaysReviewable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()
	call := env.lastCall()
	if err := os.WriteFile(filepath.Join(call.WorktreePath, "dirty.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.complete("completed")
	env.cycle()

	reviews, err := ReadPendingReview(env.loop.runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].ApprovalAction != approvalActionComplete {
		t.Fatalf("reviews = %#v", reviews)
	}
	if err := env.loop.controlMerge("42"); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("approval error = %v", err)
	}
	reviews, err = ReadPendingReview(env.loop.runtimeDir)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("review after refusal = %#v err=%v", reviews, err)
	}

	gitCommit(t, call.WorktreePath, "ready now")
	if err := env.loop.controlMerge("42"); err != nil {
		t.Fatalf("approval: %v", err)
	}
	reviews, err = ReadPendingReview(env.loop.runtimeDir)
	if err != nil || len(reviews) != 0 {
		t.Fatalf("reviews after approval = %#v err=%v", reviews, err)
	}
	if len(env.adapter.doneCalls) != 1 || env.adapter.doneCalls[0] != "42" {
		t.Fatalf("done = %#v", env.adapter.doneCalls)
	}
	if _, err := os.Stat(call.WorktreePath); err != nil {
		t.Fatalf("approved worktree missing: %v", err)
	}
	if !localBranchExists(env.dir, renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")) {
		t.Fatal("approval deleted the branch")
	}
}

func TestBranchWorkspaceRequestChangesReusesCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()
	call := env.lastCall()
	gitCommit(t, call.WorktreePath, "keep the note")
	env.complete("completed")
	env.cycle()
	if err := env.loop.controlRequestChanges("42", "tighten the copy"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(call.WorktreePath); err != nil {
		t.Fatalf("request-changes removed worktree: %v", err)
	}
	if err := env.loop.controlRequeue("42"); err != nil {
		t.Fatal(err)
	}
	env.cycle()

	again := env.lastCall()
	if again.WorktreePath != call.WorktreePath {
		t.Fatalf("requeue path = %q, want %q", again.WorktreePath, call.WorktreePath)
	}
	if !strings.Contains(again.Prompt, "resuming work") || !strings.Contains(again.Prompt, "keep the note") {
		t.Fatalf("retry prompt = %s", again.Prompt)
	}
	if currentBranch(t, again.WorktreePath) != renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login") {
		t.Fatalf("branch changed on requeue")
	}
}

func TestBranchWorkspaceFailureAndRemovalKeepCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()
	call := env.lastCall()
	env.complete("failed")
	env.cycle()
	if _, err := os.Stat(call.WorktreePath); err != nil {
		t.Fatalf("failure removed worktree: %v", err)
	}
	if ws, ok := env.loop.orderWorkspace("42"); !ok || ws.WorktreeName != call.Name {
		t.Fatalf("workspace after failure = %#v ok=%v", ws, ok)
	}

	env.loop.cooks.activeCooksByOrder = map[string]*cookHandle{}
	if err := env.loop.removeOrder("42"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(call.WorktreePath); err != nil {
		t.Fatalf("order removal removed worktree: %v", err)
	}
	if _, ok := env.loop.canonical.Orders["42"]; ok {
		t.Fatal("removed order still in canonical state")
	}
}

func TestBranchWorkspaceRejectionKeepsCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("7", "Reject me", gitStage("execute", "execute", "process")))
	env.cycle()
	call := env.lastCall()
	gitCommit(t, call.WorktreePath, "please review")
	env.complete("completed")
	env.cycle()
	if err := env.loop.controlReject("7"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(call.WorktreePath); err != nil {
		t.Fatalf("rejection removed worktree: %v", err)
	}
	if !localBranchExists(env.dir, renderBranchPattern(config.DefaultBranchPattern, "7", "Reject me")) {
		t.Fatal("rejection deleted the branch")
	}
	reviews, err := ReadPendingReview(env.loop.runtimeDir)
	if err != nil || len(reviews) != 0 {
		t.Fatalf("reviews = %#v err=%v", reviews, err)
	}
}

func TestBranchWorkspaceNoChangeCompletesWithoutReview(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()
	call := env.lastCall()
	env.complete("completed")
	env.cycle()

	reviews, err := ReadPendingReview(env.loop.runtimeDir)
	if err != nil || len(reviews) != 0 {
		t.Fatalf("reviews = %#v err=%v", reviews, err)
	}
	if len(env.adapter.doneCalls) != 1 || env.adapter.doneCalls[0] != "42" {
		t.Fatalf("done = %#v", env.adapter.doneCalls)
	}
	if _, err := os.Stat(call.WorktreePath); err != nil {
		t.Fatalf("no-change removed worktree: %v", err)
	}
	if !localBranchExists(env.dir, renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login")) {
		t.Fatal("no-change deleted the branch")
	}
}

func TestAdoptedSessionUsesOrderWorkspace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration in short mode")
	}
	env := newGitLoop(t, branchTestConfig("supervised"))
	env.setOrders(gitOrder("42", "Fix login", gitStage("execute", "execute", "process")))
	env.cycle()
	call := env.lastCall()
	gitCommit(t, call.WorktreePath, "adopted work")

	env.rt.mu.Lock()
	session := env.rt.sessions[0]
	env.rt.mu.Unlock()
	delete(env.loop.cooks.activeCooksByOrder, "42")
	env.loop.cooks.adoptedTargets = map[string]string{"42": session.ID()}
	metaDir := filepath.Join(env.loop.runtimeDir, "sessions", session.ID())
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "meta.json"), []byte(`{"status":"completed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	env.cycle()

	reviews, err := ReadPendingReview(env.loop.runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews = %#v", reviews)
	}
	if reviews[0].WorktreeName != orderWorktreeName("42") || reviews[0].WorktreePath != call.WorktreePath {
		t.Fatalf("adopted review = %#v", reviews[0])
	}
	if reviews[0].BranchName != renderBranchPattern(config.DefaultBranchPattern, "42", "Fix login") {
		t.Fatalf("adopted branch = %q", reviews[0].BranchName)
	}
}

type gitLoopEnv struct {
	t       *testing.T
	dir     string
	base    string
	app     *worktree.App
	loop    *Loop
	rt      *mockRuntime
	adapter *fakeAdapterRunner
}

func newGitLoop(t *testing.T, cfg config.Config) *gitLoopEnv {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "develop")
	git(t, dir, "config", "user.email", "test@test.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "gc.auto", "0")
	git(t, dir, "config", "core.fsmonitor", "false")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".worktrees\n.noodle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "initial")

	t.Cleanup(func() {
		_ = exec.Command("git", "-C", dir, "worktree", "prune", "--expire", "now").Run()
		_ = os.RemoveAll(filepath.Join(dir, ".git", "worktrees"))
		_ = os.RemoveAll(filepath.Join(dir, ".worktrees"))
	})

	app := &worktree.App{Root: dir, IntegrationBranch: "develop", Quiet: true}
	rt := newMockRuntime()
	adapter := &fakeAdapterRunner{}
	runtimeDir := filepath.Join(dir, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	l := New(dir, "noodle", cfg, Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": rt},
		Worktree:   app,
		Adapter:    adapter,
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:        timeNowForTests,
		OrdersFile: filepath.Join(runtimeDir, "orders.json"),
	})
	return &gitLoopEnv{t: t, dir: dir, base: "develop", app: app, loop: l, rt: rt, adapter: adapter}
}

func timeNowForTests() time.Time {
	return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
}

func branchTestConfig(mode string) config.Config {
	cfg := config.DefaultConfig()
	cfg.Mode = mode
	cfg.Git.Completion = config.GitCompletionBranch
	cfg.Git.BranchPattern = config.DefaultBranchPattern
	return cfg
}

func gitOrder(id, title string, stages ...Stage) Order {
	return Order{
		ID:     id,
		Title:  title,
		Status: OrderStatusActive,
		Stages: stages,
	}
}

func gitStage(taskKey, skill, runtime string) Stage {
	return Stage{
		TaskKey:  taskKey,
		Skill:    skill,
		Provider: "claude",
		Model:    "claude-opus-4-6",
		Runtime:  runtime,
		Status:   StageStatusPending,
	}
}

func (e *gitLoopEnv) setOrders(orders ...Order) {
	e.t.Helper()
	if err := writeOrdersAtomic(e.loop.deps.OrdersFile, OrdersFile{Orders: orders}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *gitLoopEnv) readOrders() OrdersFile {
	e.t.Helper()
	orders, err := readOrders(e.loop.deps.OrdersFile)
	if err != nil {
		e.t.Fatal(err)
	}
	return orders
}

func (e *gitLoopEnv) cycle() {
	e.t.Helper()
	if err := e.loop.Cycle(context.Background()); err != nil {
		e.t.Fatalf("cycle: %v", err)
	}
}

func (e *gitLoopEnv) complete(status string) {
	e.t.Helper()
	e.rt.mu.Lock()
	sessions := append([]*mockSession(nil), e.rt.sessions...)
	e.rt.mu.Unlock()
	n := 0
	for _, session := range sessions {
		if session.Status() == "running" {
			session.complete(status)
			n++
		}
	}
	if n == 0 {
		e.t.Fatal("no running session")
	}
}

func (e *gitLoopEnv) adopt(orderID string) {
	e.t.Helper()
	e.rt.mu.Lock()
	var session *mockSession
	for _, candidate := range e.rt.sessions {
		if candidate.Status() == "running" {
			session = candidate
		}
	}
	e.rt.mu.Unlock()
	if session == nil {
		e.t.Fatal("no running session")
	}
	delete(e.loop.cooks.activeCooksByOrder, orderID)
	if e.loop.cooks.adoptedTargets == nil {
		e.loop.cooks.adoptedTargets = map[string]string{}
	}
	e.loop.cooks.adoptedTargets[orderID] = session.ID()
	metaDir := filepath.Join(e.loop.runtimeDir, "sessions", session.ID())
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "meta.json"), []byte(`{"status":"completed"}`), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *gitLoopEnv) emitStageMessage(message string, blocking bool) {
	e.t.Helper()
	e.rt.mu.Lock()
	sessionID := ""
	for _, session := range e.rt.sessions {
		if session.Status() == "running" {
			sessionID = session.ID()
		}
	}
	e.rt.mu.Unlock()
	if sessionID == "" {
		e.t.Fatal("no running session")
	}
	writer, err := event.NewEventWriter(e.loop.runtimeDir, sessionID)
	if err != nil {
		e.t.Fatal(err)
	}
	payload, err := json.Marshal(event.StageMessagePayload{Message: message, Blocking: &blocking})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := writer.Append(context.Background(), event.Event{
		Type:      event.EventStageMessage,
		Payload:   payload,
		SessionID: sessionID,
	}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *gitLoopEnv) lastCall() loopruntime.DispatchRequest {
	e.t.Helper()
	e.rt.mu.Lock()
	defer e.rt.mu.Unlock()
	if len(e.rt.calls) == 0 {
		e.t.Fatal("no dispatch")
	}
	return e.rt.calls[len(e.rt.calls)-1]
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "change.txt"), []byte(message+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", message)
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func localBranchExists(dir, branch string) bool {
	cmd := exec.Command("git", "-C", dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return cmd.Run() == nil
}

func gitLogContains(t *testing.T, dir, message string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "--format=%s")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == message {
			return true
		}
	}
	return false
}

func countBranchCheckouts(t *testing.T, dir, branch string) int {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	want := "branch refs/heads/" + branch
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == want {
			n++
		}
	}
	return n
}
