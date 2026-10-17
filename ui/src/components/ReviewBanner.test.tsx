import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewBanner } from "./ReviewBanner";
import { buildReview } from "~/test-utils";

const mockMutate = vi.fn();
const controlState = { isPending: false };

vi.mock("~/client", () => ({
  useSendControl: () => ({ mutate: mockMutate, isPending: controlState.isPending }),
}));

describe("ReviewBanner", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    controlState.isPending = false;
  });

  it("labels a completion review as complete", () => {
    render(<ReviewBanner review={buildReview({ order_id: "42", approval_action: "complete" })} />);

    expect(screen.getByRole("button", { name: "COMPLETE" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "MERGE" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "COMPLETE" }));
    expect(mockMutate).toHaveBeenCalledWith({ action: "merge", order_id: "42" });
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
});
