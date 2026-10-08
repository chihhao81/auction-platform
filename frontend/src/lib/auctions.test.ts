import { afterEach, describe, expect, it, vi } from "vitest";
import { createAuction, placeBid, uploadAuctionImage, type AuctionInput } from "./auctions";

afterEach(() => vi.unstubAllGlobals());

const input: AuctionInput = {
  title: "豹紋守宮", description: "健康個體", starting_price: 0,
  min_increment: 50, max_increment: 500,
  starts_at: "2026-01-01T12:00:00.000Z", ends_at: "2026-01-03T12:00:00.000Z",
  contact_method: "", shipping_method: "店到店", shipping_fee: 60, images: [],
};

describe("auction API", () => {
  it("creates an auction using the backend payload and session cookie", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(Response.json({ id: 8 }, { status: 201 }));
    vi.stubGlobal("fetch", fetcher);
    await createAuction(input);
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/v1/auctions"), expect.objectContaining({
      method: "POST", credentials: "include", body: JSON.stringify(input),
    }));
  });

  it("sends the submitted amount for backend validation", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(Response.json({ auction_id: 8, current_price: 1050, ends_at: input.ends_at }, { status: 201 }));
    vi.stubGlobal("fetch", fetcher);
    await placeBid("8", 1050);
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/v1/auctions/8/bids"), expect.objectContaining({ body: JSON.stringify({ amount: 1050 }) }));
  });

  it("uploads the file as multipart without overriding the boundary header", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(Response.json({ id: "a".repeat(64), url: `/v1/uploads/${"a".repeat(64)}` }, { status: 201 }));
    vi.stubGlobal("fetch", fetcher);
    const file = new File([new Uint8Array([1, 2, 3])], "animal.png", { type: "image/png" });
    await uploadAuctionImage(file);
    const [, options] = fetcher.mock.calls[0];
    expect(options?.method).toBe("POST");
    expect(options?.credentials).toBe("include");
    expect(options?.body).toBeInstanceOf(FormData);
    expect(new Headers(options?.headers).has("Content-Type")).toBe(false);
  });
});
