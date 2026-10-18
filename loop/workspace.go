package loop

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
	"github.com/poteto/noodle/internal/stringx"
)

const (
	approvalActionComplete = "complete"
	approvalActionContinue = "continue"
)

func (l *Loop) branchModeEnabled() bool {
	return l.config.Git.Completion == config.GitCompletionBranch
}

func (l *Loop) usesBranchWorkspace(order Order, worktreeName string) bool {
	worktreeName = strings.TrimSpace(worktreeName)
	if ws, ok := l.orderWorkspace(order.ID); ok {
		return worktreeName == "" || worktreeName == ws.WorktreeName
	}
	if worktreeName != "" {
		return false
	}
	if node, ok := l.canonical.Orders[strings.TrimSpace(order.ID)]; ok {
		for _, stage := range node.Stages {
			if len(stage.Attempts) > 0 {
				return false
			}
		}
	}
	if !l.branchModeEnabled() {
		return false
	}
	return !isScheduleBootstrapOrder(order)
}

func branchRuntimeError(order Order) error {
	for i, stage := range order.Stages {
		runtime := stringx.Normalize(stage.Runtime)
		if runtime == "" || runtime == "process" {
			continue
		}
		return fmt.Errorf("order %s stage %d runtime is %q", order.ID, i, runtime)
	}
	return nil
}

func (l *Loop) orderWorkspace(orderID string) (state.OrderWorkspace, bool) {
	order, ok := l.canonical.Orders[strings.TrimSpace(orderID)]
	if !ok || order.Workspace == nil {
		return state.OrderWorkspace{}, false
	}
	ws := *order.Workspace
	if strings.TrimSpace(ws.WorktreeName) == "" || strings.TrimSpace(ws.BranchName) == "" || strings.TrimSpace(ws.BaseBranch) == "" {
		return state.OrderWorkspace{}, false
	}
	return ws, true
}

func (l *Loop) usesOrderWorkspace(orderID, worktreeName string) (state.OrderWorkspace, bool) {
	ws, ok := l.orderWorkspace(orderID)
	if !ok || strings.TrimSpace(worktreeName) != ws.WorktreeName {
		return state.OrderWorkspace{}, false
	}
	return ws, true
}

func (l *Loop) ensureOrderWorkspace(order Order) (state.OrderWorkspace, error) {
	if err := l.ensureCanonicalOrderFromOrders(order.ID); err != nil {
		return state.OrderWorkspace{}, err
	}
	ws, bound := l.orderWorkspace(order.ID)
	if !bound {
		base, err := l.deps.Worktree.CurrentBranch()
		if err != nil {
			return state.OrderWorkspace{}, err
		}
		ws = state.OrderWorkspace{
			WorktreeName: orderWorktreeName(order.ID),
			BranchName:   renderBranchPattern(l.config.Git.BranchPattern, order.ID, order.Title),
			BaseBranch:   base,
		}
	}
	if err := config.ValidateBranchRef(ws.BranchName); err != nil {
		return state.OrderWorkspace{}, err
	}
	if err := l.rejectSharedWorkspace(order.ID, ws); err != nil {
		return state.OrderWorkspace{}, err
	}
	if err := l.materializeWorkspace(ws); err != nil {
		return state.OrderWorkspace{}, err
	}
	if !bound {
		if err := l.storeOrderWorkspace(order.ID, ws); err != nil {
			return state.OrderWorkspace{}, err
		}
	}
	return ws, nil
}

func (l *Loop) rejectSharedWorkspace(orderID string, ws state.OrderWorkspace) error {
	for id, node := range l.canonical.Orders {
		if id == orderID || node.Workspace == nil {
			continue
		}
		other := *node.Workspace
		if ws.WorktreeName != "" && other.WorktreeName == ws.WorktreeName {
			return fmt.Errorf("worktree name %s is already used by order %s", ws.WorktreeName, id)
		}
		if ws.BranchName != "" && other.BranchName == ws.BranchName {
			return fmt.Errorf("branch %s is already used by order %s", ws.BranchName, id)
		}
	}
	return nil
}

func (l *Loop) storeOrderWorkspace(orderID string, ws state.OrderWorkspace) error {
	order, ok := l.canonical.Orders[orderID]
	if !ok {
		return fmt.Errorf("order %s missing", orderID)
	}
	switch order.Status {
	case state.OrderActive, state.OrderPending, "":
	default:
		return fmt.Errorf("order %s is %s", orderID, order.Status)
	}
	copy := ws
	order.Workspace = &copy
	order.UpdatedAt = timeNowUTC(l.deps.Now)
	l.canonical.Orders[orderID] = order
	return l.persistCanonicalCheckpoint()
}

func (l *Loop) materializeWorkspace(ws state.OrderWorkspace) error {
	path := l.worktreePath(ws.WorktreeName)
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("worktree path %s is not a directory", path)
		}
		branch, err := l.deps.Worktree.CheckedOutBranch(path)
		if err != nil {
			return err
		}
		if branch != ws.BranchName {
			if branch == "" {
				return fmt.Errorf("worktree %s at %s is detached, expected branch %s", ws.WorktreeName, path, ws.BranchName)
			}
			return fmt.Errorf("worktree %s at %s is on branch %s, expected %s", ws.WorktreeName, path, branch, ws.BranchName)
		}
		return nil
	case os.IsNotExist(err):
	default:
		return fmt.Errorf("stat worktree path %s: %w", path, err)
	}

	if l.deps.Worktree.BranchExists(ws.BranchName) {
		attached, ok, err := l.deps.Worktree.BranchWorktree(ws.BranchName)
		if err != nil {
			return err
		}
		if ok {
			return fmt.Errorf("branch %s is checked out at %s", ws.BranchName, attached)
		}
		return l.deps.Worktree.AttachBranch(ws.WorktreeName, ws.BranchName)
	}
	return l.deps.Worktree.CreateBranch(ws.WorktreeName, ws.BranchName, ws.BaseBranch)
}

func (l *Loop) shouldResumeBranchStage(orderID string, stageIndex, attempt int) bool {
	if attempt > 0 {
		return true
	}
	order, ok := l.canonical.Orders[orderID]
	if !ok || stageIndex < 0 || stageIndex >= len(order.Stages) {
		return false
	}
	return len(order.Stages[stageIndex].Attempts) > 0
}

func (l *Loop) stageIsFinal(cook *cookHandle) bool {
	order, ok := l.canonical.Orders[cook.orderID]
	if !ok || len(order.Stages) == 0 {
		return true
	}
	return cook.stageIndex >= len(order.Stages)-1
}

func (l *Loop) completeBranchWorkspace(ctx context.Context, cook *cookHandle, ws state.OrderWorkspace, msg *string) error {
	if !l.stageIsFinal(cook) {
		if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, false)); err != nil {
			return err
		}
		return l.advanceAndPersist(ctx, cook, msg)
	}

	clean, err := l.deps.Worktree.Clean(cook.worktreePath)
	if err != nil {
		return l.failStage(ctx, cook, fmt.Sprintf("worktree status: %v", err))
	}
	ahead, err := l.deps.Worktree.CommitsAhead(ws.BranchName, ws.BaseBranch)
	if err != nil {
		return l.failStage(ctx, cook, fmt.Sprintf("commit check: %v", err))
	}
	if clean && !ahead {
		if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, false)); err != nil {
			return err
		}
		return l.advanceAndPersist(ctx, cook, msg)
	}

	reason := "ready for review"
	if !clean {
		reason = "worktree has uncommitted changes"
	}
	return l.parkPendingReview(cook, reason)
}

func (l *Loop) approveBranchWorkspace(pending *pendingReviewCook) error {
	path := strings.TrimSpace(pending.worktreePath)
	if path == "" {
		path = l.worktreePath(pending.worktreeName)
	}
	clean, err := l.deps.Worktree.Clean(path)
	if err != nil {
		return fmt.Errorf("worktree status: %w", err)
	}
	if !clean {
		return fmt.Errorf("worktree %s has uncommitted changes", pending.worktreeName)
	}

	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		orderStatus:  OrderStatusActive,
		worktreeName: pending.worktreeName,
		worktreePath: path,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	return l.finishWorkspaceReview(cook)
}

func (l *Loop) continueWorkspaceStage(pending *pendingReviewCook) error {
	path := strings.TrimSpace(pending.worktreePath)
	if path == "" {
		path = l.worktreePath(pending.worktreeName)
	}
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		orderStatus:  OrderStatusActive,
		worktreeName: pending.worktreeName,
		worktreePath: path,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	return l.finishWorkspaceReview(cook)
}

func (l *Loop) finishWorkspaceReview(cook *cookHandle) error {
	if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, false)); err != nil {
		return err
	}
	if err := l.advanceAndPersist(context.Background(), cook); err != nil {
		return err
	}
	if err := l.emitEventChecked(ingest.EventStageReviewApproved, map[string]any{
		"order_id":    cook.orderID,
		"stage_index": cook.stageIndex,
		"reason":      "approved by user",
	}); err != nil {
		return err
	}
	return l.syncPendingReviewProjection()
}
