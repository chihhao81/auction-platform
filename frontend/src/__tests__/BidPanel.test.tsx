import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BidPanel, bidBlockReason } from "@/components/BidPanel";
import type { AuctionDetail } from "@/lib/types";

const auction = (over: Partial<AuctionDetail> = {}): AuctionDetail => ({
  id: "a1",
  title: "gecko",
  images: [],
  current_price: 1000,
  bid_count: 1,
  start_time: "2026-01-01T00:00:00Z",
  end_time: "2026-01-02T00:00:00Z",
  status: "ACTIVE",
  seller_name: "s",
  description: "",
  start_price: 0,
  min_increment: 50,
  max_increment: 500,
  original_end_time: "2026-01-02T00:00:00Z",
  shipping_method: "post",
  shipping_fee: 0,
  seller: { id: "s", display_name: "s", avatar_url: "" },
  bids: [],
  next_bid_min: 1050,
  next_bid_max: 1500,
  is_seller: false,
  is_highest_bidder: false,
  ...over,
});

describe("bidBlockReason", () => {
  it("mirrors backend bid rules", () => {
    expect(bidBlockReason(auction({ status: "UPCOMING" }), true)).toBe("競標尚未開始");
    expect(bidBlockReason(auction({ status: "ENDED" }), true)).toBe("競標已結束");
    expect(bidBlockReason(auction(), false)).toBe("LOGIN");
    expect(bidBlockReason(auction({ is_seller: true }), true)).toContain("賣家");
    expect(bidBlockReason(auction({ is_highest_bidder: true }), true)).toContain("最高出價者");
    expect(bidBlockReason(auction(), true)).toBeNull();
  });
});

describe("BidPanel", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("blocks out-of-range bids client side without calling the API", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    render(<BidPanel auction={auction()} loggedIn onBid={() => {}} />);
    const input = screen.getByLabelText("出價金額");
    await userEvent.clear(input);
    await userEvent.type(input, "2000");
    await userEvent.click(screen.getByRole("button", { name: "出價" }));
    expect(screen.getByRole("alert")).toHaveTextContent("1,050 ~ 1,500");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("submits a valid bid and reports extension", async () => {
    document.cookie = "csrf_token=t";
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ bid_id: "b", current_price: 1100, bid_count: 2, end_time: "", extended: true, next_bid_min: 1150, next_bid_max: 1600 }),
        { status: 201 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const onBid = vi.fn();
    render(<BidPanel auction={auction()} loggedIn onBid={onBid} />);
    const input = screen.getByLabelText("出價金額");
    await userEvent.clear(input);
    await userEvent.type(input, "1100");
    await userEvent.click(screen.getByRole("button", { name: "出價" }));
    expect(await screen.findByRole("status")).toHaveTextContent("延長 2 分鐘");
    expect(fetchMock).toHaveBeenCalledWith("/api/auctions/a1/bids", expect.objectContaining({ method: "POST", body: '{"amount":1100}' }));
    expect(onBid).toHaveBeenCalled();
  });

  it("shows backend rejection messages", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: { code: "BID_OUT_OF_RANGE", message: "出價需介於 1100 ~ 1550" } }), { status: 400 }),
      ),
    );
    render(<BidPanel auction={auction()} loggedIn onBid={() => {}} />);
    await userEvent.click(screen.getByRole("button", { name: "出價" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("出價需介於 1100 ~ 1550");
  });

  it("asks anonymous users to log in", () => {
    render(<BidPanel auction={auction()} loggedIn={false} onBid={() => {}} />);
    expect(screen.getByRole("link", { name: "登入後出價" })).toHaveAttribute("href", "/login?next=%2Fauctions%2Fa1");
  });
});
