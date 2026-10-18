import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewList } from "./ReviewList";
import { buildReview, buildSnapshot } from "~/test-utils";

let mockSnapshot = buildSnapshot();
const mockMutate = vi.fn();
const diffState = { uncommitted: false };

vi.mock("~/client", async () => {
  const actual = await vi.importActual("~/client");
  return {
    ...(actual as Record<string, unknown>),
    useSuspenseSnapshot: () => ({ data: mockSnapshot }),
    useSendControl: () => ({ mutate: mockMutate, isPending: false }),
    useReviewDiff: () => ({
      data: { diff: "", stat: "", uncommitted: diffState.uncommitted },
      isLoading: false,
      isFetching: false,
      error: undefined,
      refetch: vi.fn(),
    }),
  };
});

describe("ReviewList", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockSnapshot = buildSnapshot();
    diffState.uncommitted = false;
  });

  it("keeps a valid selection when reviews shrink", () => {
    const first = buildReview({ order_id: "o-1", title: "First" });
    const second = buildReview({ order_id: "o-2", title: "Second" });
    mockSnapshot = buildSnapshot({
      pending_reviews: [first, second],
      pending_review_count: 2,
    });

    const view = render(<ReviewList />);
    fireEvent.click(screen.getByRole("tab", { name: "Second" }));

    mockSnapshot = buildSnapshot({
      pending_reviews: [first],
      pending_review_count: 1,
    });
    view.rerender(<ReviewList />);

    expect(screen.getByText("Review Actions")).toBeInTheDocument();
    expect(screen.getByText(/Quality Review:/)).toBeInTheDocument();
    expect(screen.getByText("o-1")).toBeInTheDocument();
  });

  it("labels a branch workspace review as complete and shows its branch", () => {
    mockSnapshot = buildSnapshot({
      pending_reviews: [
        buildReview({
          order_id: "42",
          approval_action: "complete",
          branch_name: "noodle/42-fix-login",
          base_branch: "develop",
        }),
      ],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Approve for Handoff" })).toBeInTheDocument();
    expect(screen.getByText("noodle/42-fix-login")).toBeInTheDocument();
    expect(screen.getByText("develop")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Approve for Handoff" }));
    expect(mockMutate).toHaveBeenCalledWith({ action: "merge", order_id: "42" });
  });

  it("shows dirty work and blocks handoff approval until committed", () => {
    diffState.uncommitted = true;
    mockSnapshot = buildSnapshot({
      pending_reviews: [buildReview({ approval_action: "complete" })],
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Approve for Handoff" })).toBeDisabled();
    expect(screen.getByText(/This worktree has uncommitted changes/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Refresh Diff" })).toBeInTheDocument();
  });

  it("labels a branch workspace review as continue", () => {
    mockSnapshot = buildSnapshot({
      pending_reviews: [
        buildReview({
          order_id: "42",
          approval_action: "continue",
          branch_name: "noodle/42-fix-login",
        }),
      ],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Approve & Continue" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Approve & Merge" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Approve & Continue" }));
    expect(mockMutate).toHaveBeenCalledWith({ action: "merge", order_id: "42" });
  });

  it("keeps merge approval copy when the review has no completion action", () => {
    mockSnapshot = buildSnapshot({
      pending_reviews: [buildReview({ order_id: "7" })],
      pending_review_count: 1,
    });

    render(<ReviewList />);

    expect(screen.getByRole("button", { name: "Approve & Merge" })).toBeInTheDocument();
    expect(screen.queryByText("Branch")).not.toBeInTheDocument();
  });
});
