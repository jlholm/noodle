import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewBanner } from "./ReviewBanner";
import { buildReview } from "~/test-utils";

const mockMutate = vi.fn();
const controlState = { isPending: false };
const diffState = { uncommitted: false, available: true };

vi.mock("~/client", () => ({
  useSendControl: () => ({ mutate: mockMutate, isPending: controlState.isPending }),
  useReviewDiff: () => ({
    data: diffState.available
      ? { diff: "", stat: "", uncommitted: diffState.uncommitted }
      : undefined,
  }),
}));

describe("ReviewBanner", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    controlState.isPending = false;
    diffState.uncommitted = false;
    diffState.available = true;
  });

  it("labels a completion review as a handoff", () => {
    render(<ReviewBanner review={buildReview({ order_id: "42", approval_action: "complete" })} />);

    expect(screen.getByRole("button", { name: "APPROVE HANDOFF" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "MERGE" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "APPROVE HANDOFF" }));
    expect(mockMutate).toHaveBeenCalledWith({ action: "merge", order_id: "42" });
  });

  it("requires a commit before handoff approval", () => {
    diffState.uncommitted = true;
    render(<ReviewBanner review={buildReview({ approval_action: "complete" })} />);

    expect(screen.getByRole("button", { name: "APPROVE HANDOFF" })).toBeDisabled();
    expect(screen.getByText("Commit worktree changes before approval.")).toBeInTheDocument();
  });

  it("requires the review diff to load before handoff approval", () => {
    diffState.available = false;
    render(<ReviewBanner review={buildReview({ approval_action: "complete" })} />);

    expect(screen.getByRole("button", { name: "APPROVE HANDOFF" })).toBeDisabled();
    expect(screen.getByText(/Review diff unavailable/)).toBeInTheDocument();
  });

  it("labels a completion review as completing while the control is in flight", () => {
    controlState.isPending = true;
    render(<ReviewBanner review={buildReview({ approval_action: "complete" })} />);

    expect(screen.getByRole("button", { name: "COMPLETING…" })).toBeInTheDocument();
  });

  it("keeps the merge label when the review does not complete the order", () => {
    render(<ReviewBanner review={buildReview({ order_id: "7" })} />);

    expect(screen.getByRole("button", { name: "MERGE" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "MERGE" }));
    expect(mockMutate).toHaveBeenCalledWith({ action: "merge", order_id: "7" });
  });

  it("labels a continue review as continue", () => {
    render(<ReviewBanner review={buildReview({ order_id: "42", approval_action: "continue" })} />);

    expect(screen.getByRole("button", { name: "CONTINUE" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "MERGE" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "CONTINUE" }));
    expect(mockMutate).toHaveBeenCalledWith({ action: "merge", order_id: "42" });
  });

  it("labels a continue review as continuing while the control is in flight", () => {
    controlState.isPending = true;
    render(<ReviewBanner review={buildReview({ approval_action: "continue" })} />);

    expect(screen.getByRole("button", { name: "CONTINUING…" })).toBeInTheDocument();
  });
});
