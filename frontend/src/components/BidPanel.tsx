"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { ApiError, api } from "@/lib/api";
import { bidAmountError, formatPrice, quickBids } from "@/lib/auction";
import type { AuctionDetail, BidResponse } from "@/lib/types";

export type Viewer = "anonymous" | "needs_terms" | "member";

interface Props {
  auction: AuctionDetail;
  viewer: Viewer;
  onBid: (res: BidResponse) => void;
}

// Reasons the viewer cannot bid, mirroring backend rules for UX only.
export function bidBlockReason(a: AuctionDetail, viewer: Viewer): string | null {
  if (a.status === "UPCOMING") return "競標尚未開始";
  if (a.status !== "ACTIVE") return "競標已結束";
  if (viewer === "anonymous") return "LOGIN";
  if (viewer === "needs_terms") return "TERMS";
  if (a.is_seller) return "您是此商品的賣家，無法出價";
  if (a.is_highest_bidder) return "您目前是最高出價者";
  return null;
}

export function BidPanel({ auction, viewer, onBid }: Props) {
  const [amount, setAmount] = useState(String(auction.next_bid_min));
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const block = bidBlockReason(auction, viewer);
  const min = auction.next_bid_min;
  const max = auction.next_bid_max;

  async function submit(e: FormEvent) {
    e.preventDefault();
    const err = bidAmountError(amount, min, max);
    setMessage(null);
    if (err) {
      setError(err);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const res = await api.post<BidResponse>(`/api/auctions/${auction.id}/bids`, { amount: Number(amount) });
      setMessage(res.extended ? "出價成功！競標時間已延長 2 分鐘" : "出價成功！");
      setAmount(String(res.next_bid_min));
      onBid(res);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "出價失敗，請稍後再試");
    } finally {
      setSubmitting(false);
    }
  }

  if (block === "LOGIN") {
    return (
      <Link
        href={`/login?next=${encodeURIComponent(`/auctions/${auction.id}`)}`}
        className="block rounded bg-emerald-600 px-4 py-2.5 text-center font-medium text-white"
      >
        登入後出價
      </Link>
    );
  }
  if (block === "TERMS") {
    return (
      <Link
        href={`/terms?next=${encodeURIComponent(`/auctions/${auction.id}`)}`}
        className="block rounded bg-emerald-600 px-4 py-2.5 text-center font-medium text-white"
      >
        同意使用者條款後出價
      </Link>
    );
  }
  if (block) return <p className="rounded bg-gray-100 p-3 text-center text-sm text-gray-600">{block}</p>;

  return (
    <form onSubmit={submit} className="space-y-2">
      <p className="text-sm text-gray-600">
        可出價範圍 {formatPrice(min)} ~ {formatPrice(max)}
      </p>
      <div className="flex flex-wrap gap-2">
        {quickBids(min, max).map((v) => (
          <button
            type="button"
            key={v}
            onClick={() => setAmount(String(v))}
            className="rounded border px-2 py-1 text-sm hover:bg-gray-50"
          >
            {formatPrice(v)}
          </button>
        ))}
      </div>
      <div className="flex gap-2">
        <input
          inputMode="numeric"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          className="min-w-0 flex-1 rounded border px-3 py-2"
          aria-label="出價金額"
        />
        <button
          disabled={submitting}
          className="rounded bg-emerald-600 px-5 py-2 font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
        >
          {submitting ? "出價中…" : "出價"}
        </button>
      </div>
      {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
      {message && <p role="status" className="text-sm text-emerald-700">{message}</p>}
    </form>
  );
}
