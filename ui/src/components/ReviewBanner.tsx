import { useState } from "react";
import { useReviewDiff, useSendControl } from "~/client";
import type { PendingReviewItem } from "~/client";

export function ReviewBanner({ review }: { review: PendingReviewItem }) {
  const { mutate: send, isPending } = useSendControl();
  const { data: diff } = useReviewDiff(review.order_id);
  const [showFeedback, setShowFeedback] = useState(false);
  const [feedback, setFeedback] = useState("");

  function handleMerge() {
    send({ action: "merge", order_id: review.order_id });
  }

  function handleReject() {
    send({ action: "reject", order_id: review.order_id });
  }

  function handleRequestChanges() {
    if (!showFeedback) {
      setShowFeedback(true);
      return;
    }
    send({
      action: "request-changes",
      order_id: review.order_id,
      prompt: feedback,
    });
    setShowFeedback(false);
    setFeedback("");
  }

  let requestChangesLabel = "REQUEST CHANGES";
  if (isPending) {
    requestChangesLabel = "SENDING…";
  } else if (showFeedback) {
    requestChangesLabel = "SEND";
  }

  let approveLabel = "MERGE";
  let approvePendingLabel = "MERGING…";
  if (review.approval_action === "complete") {
    approveLabel = "APPROVE HANDOFF";
    approvePendingLabel = "COMPLETING…";
  } else if (review.approval_action === "continue") {
    approveLabel = "CONTINUE";
    approvePendingLabel = "CONTINUING…";
  }

  return (
    <div className="border-l-2 border-accent p-4 bg-accent/5 mx-4 mb-2">
      <div className="text-xs font-display font-bold uppercase tracking-wider text-accent mb-1">
        Ready for review
      </div>
      {review.reason && (
        <div className="text-xs font-body text-neutral-400 mb-3">{review.reason}</div>
      )}
      {review.approval_action === "complete" && diff?.uncommitted && (
        <div className="text-xs font-body text-accent mb-3">
          Commit worktree changes before approval.
        </div>
      )}
      {review.approval_action === "complete" && !diff && (
        <div className="text-xs font-body text-accent mb-3">
          Review diff unavailable. Open Reviews to retry.
        </div>
      )}

      {showFeedback && (
        <div className="mb-3">
          <input
            type="text"
            className="w-full px-2 py-1.5 font-body text-xs border border-border-subtle bg-transparent text-text-primary outline-none focus:border-accent"
            placeholder="What needs to change?"
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                handleRequestChanges();
              }
              if (e.key === "Escape") {
                setShowFeedback(false);
              }
            }}
            autoFocus
          />
        </div>
      )}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={handleMerge}
          disabled={
            isPending || (review.approval_action === "complete" && (!diff || diff.uncommitted))
          }
          className="font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 bg-green text-black"
        >
          {isPending ? approvePendingLabel : approveLabel}
        </button>
        <button
          type="button"
          onClick={handleReject}
          disabled={isPending}
          className="font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 bg-red text-white"
        >
          {isPending ? "REJECTING…" : "REJECT"}
        </button>
        <button
          type="button"
          onClick={handleRequestChanges}
          disabled={isPending}
          className="font-body font-bold uppercase text-[10px] tracking-wider px-3 py-1.5 border border-accent text-accent"
        >
          {requestChangesLabel}
        </button>
      </div>
    </div>
  );
}
