import { describe, expect, it } from "vitest";
import { applyPrices, bidAmountError, clockOffset, formatPrice, quickBids } from "@/lib/auction";
import { defaultValues, toPayload, validateForm, type FormValues } from "@/components/AuctionForm";
import type { AuctionSummary } from "@/lib/types";

describe("bidAmountError", () => {
  it("accepts the inclusive range", () => {
    expect(bidAmountError("1050", 1050, 1500)).toBeNull();
    expect(bidAmountError("1500", 1050, 1500)).toBeNull();
  });
  it("rejects out of range and non-integers", () => {
    expect(bidAmountError("1049", 1050, 1500)).toContain("1,050");
    expect(bidAmountError("1501", 1050, 1500)).not.toBeNull();
    expect(bidAmountError("10.5", 1, 100)).toBe("請輸入整數金額");
    expect(bidAmountError("-5", 1, 100)).toBe("請輸入整數金額");
    expect(bidAmountError("", 1, 100)).toBe("請輸入整數金額");
  });
});

describe("quickBids", () => {
  it("spreads suggestions across the range", () => {
    expect(quickBids(1050, 1500)).toEqual([1050, 1275, 1500]);
    expect(quickBids(50, 50)).toEqual([50]);
  });
});

const item = (id: string, price: number): AuctionSummary => ({
  id,
  title: id,
  images: [],
  current_price: price,
  bid_count: 0,
  start_time: "2026-01-01T00:00:00Z",
  end_time: "2026-01-02T00:00:00Z",
  status: "ACTIVE",
  seller_name: "s",
});

describe("applyPrices", () => {
  it("updates prices and drops auctions missing from the poll (closed)", () => {
    const out = applyPrices(
      [item("a", 100), item("b", 200)],
      [{ id: "a", current_price: 150, bid_count: 1, end_time: "2026-01-02T00:02:00Z", status: "ACTIVE" }],
    );
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({ id: "a", current_price: 150, bid_count: 1, end_time: "2026-01-02T00:02:00Z" });
  });
});

describe("clockOffset", () => {
  it("measures server minus local time", () => {
    expect(clockOffset("2026-01-01T00:00:10Z", Date.parse("2026-01-01T00:00:00Z"))).toBe(10_000);
    expect(clockOffset("garbage", 0)).toBe(0);
  });
});

describe("formatPrice", () => {
  it("formats NT dollars", () => {
    expect(formatPrice(1234567)).toBe("NT$ 1,234,567");
  });
});

describe("auction form", () => {
  it("defaults to next full hour (Taipei) and 48h duration", () => {
    // 2026-10-21 12:34 UTC == 20:34 Taipei -> next hour 21:00 Taipei
    const v = defaultValues(Date.parse("2026-10-21T12:34:00Z"));
    expect(v.start_time).toBe("2026-10-21T21:00");
    expect(v.end_time).toBe("2026-10-23T21:00");
    expect([v.start_price, v.min_increment, v.max_increment]).toEqual(["0", "50", "50"]);
  });

  const valid = (): FormValues => ({
    ...defaultValues(Date.parse("2026-10-21T12:34:00Z")),
    title: "守宮",
    shipping_method: "黑貓",
  });

  it("validates required fields and ranges", () => {
    expect(validateForm(valid(), 1)).toBeNull();
    expect(validateForm({ ...valid(), title: " " }, 1)).toBe("請填寫商品名稱");
    expect(validateForm({ ...valid(), max_increment: "10" }, 1)).toBe("最高加價不可小於最小加價");
    expect(validateForm({ ...valid(), start_price: "-1" }, 1)).toContain("非負整數");
    expect(validateForm({ ...valid(), end_time: valid().start_time }, 1)).toBe("結束時間需晚於開始時間");
    expect(validateForm(valid(), 0)).toContain("圖片");
    expect(validateForm(valid(), 5)).toContain("圖片");
  });

  it("converts Taipei input to UTC payload", () => {
    const p = toPayload(valid());
    expect(p.start_time).toBe("2026-10-21T13:00:00.000Z");
    expect(p.min_increment).toBe(50);
  });
});
