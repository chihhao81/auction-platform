import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { reputationSummary } from "@/lib/reputation";
import { ReasonAction } from "@/app/admin/ReasonAction";
import { OrderReview } from "@/app/orders/[id]/OrderReview";
import type { OrderDetail } from "@/lib/types";

describe("reputationSummary", () => {
  it("formats the positive ratio", () => {
    expect(reputationSummary({ positive: 0, negative: 0, completed_orders: 3 })).toBeNull();
    expect(reputationSummary({ positive: 49, negative: 1, completed_orders: 50 })).toBe("98% 正評（49/50）");
    expect(reputationSummary({ positive: 0, negative: 2, completed_orders: 2 })).toBe("0% 正評（0/2）");
  });
});

describe("ReasonAction", () => {
  it("requires a reason before calling the action", async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const onDone = vi.fn();
    render(<ReasonAction label="停權" danger onConfirm={onConfirm} onDone={onDone} />);
    await userEvent.click(screen.getByRole("button", { name: "停權" }));
    await userEvent.click(screen.getByRole("button", { name: "確認停權" }));
    expect(screen.getByRole("alert")).toHaveTextContent("請填寫原因");
    expect(onConfirm).not.toHaveBeenCalled();

    await userEvent.type(screen.getByLabelText("停權原因"), "  詐騙  ");
    await userEvent.click(screen.getByRole("button", { name: "確認停權" }));
    expect(onConfirm).toHaveBeenCalledWith("詐騙");
    expect(onDone).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "停權" })).toBeInTheDocument();
  });
});

const order = (over: Partial<OrderDetail>): OrderDetail =>
  ({
    id: "o1",
    status: "COMPLETED",
    role: "buyer",
    my_review: null,
    their_review: null,
    ...over,
  }) as OrderDetail;

describe("OrderReview", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("is hidden until the order is completed", () => {
    const { container } = render(<OrderReview order={order({ status: "RECEIVED" })} onChanged={() => {}} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("submits a review of the seller", async () => {
    document.cookie = "csrf_token=t";
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);
    const onChanged = vi.fn();
    render(<OrderReview order={order({})} onChanged={onChanged} />);
    expect(screen.getByText(/評價賣家/)).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText("👎 負評"));
    await userEvent.type(screen.getByLabelText("評價留言"), "回覆很慢");
    await userEvent.click(screen.getByRole("button", { name: "送出評價" }));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/orders/o1/reviews",
      expect.objectContaining({ method: "POST", body: '{"rating":"NEGATIVE","comment":"回覆很慢"}' }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("shows existing reviews instead of the form", () => {
    render(
      <OrderReview
        order={order({
          role: "seller",
          my_review: { rating: "POSITIVE", comment: "好買家", created_at: "" },
          their_review: { rating: "NEGATIVE", comment: "", created_at: "" },
        })}
        onChanged={() => {}}
      />,
    );
    expect(screen.getByText("我對買家的評價")).toBeInTheDocument();
    expect(screen.getByText("買家對我的評價")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "送出評價" })).toBeNull();
  });
});
