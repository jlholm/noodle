# Branch Review Must Fail Closed

- A branch-mode order stores its workspace binding in canonical `OrderNode.Workspace` before Git creates the branch or worktree.
- The binding records `BaseBranch` for handoff and immutable `BaseCommit` for creation, change detection, and review diffs.
- Approval, rejection, and requested changes for a review marked `complete` or `continue` require a valid workspace binding and the expected checked-out branch. A missing binding must not route approval into merge or let rejection clean up the branch worktree.
- Persisted workspaces without `BaseCommit` stay recognizable as branch workspaces. Review approval fails with `workspace base commit missing` instead of merging into the integration branch.
- Order and pending-review metadata reach the UI through `internal/projection`, then `internal/snapshot`. Update both paths when changing handoff fields.
