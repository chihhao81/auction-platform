"use client";

import { useState, type FormEvent } from "react";
import { ApiError, api } from "@/lib/api";
import type { OrderDetail, OrderReview as Review } from "@/lib/types";

function ReviewLine({ label, review }: { label: string; review: Review }) {
  return (
    <div className="text-sm">
      <p className="text-gray-500">{label}</p>
      <p className={review.rating === "POSITIVE" ? "text-emerald-700" : "text-red-600"}>
        {review.rating === "POSITIVE" ? "👍 正評" : "👎 負評"}
      </p>
      {review.comment && <p className="whitespace-pre-wrap break-words">{review.comment}</p>}
    </div>
  );
}

// Two-way review, available once the order is COMPLETED.
export function OrderReview({ order, onChanged }: { order: OrderDetail; onChanged: () => void }) {
  const [rating, setRating] = useState<"POSITIVE" | "NEGATIVE">("POSITIVE");
  const [comment, setComment] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (order.status !== "COMPLETED") return null;
  const other = order.role === "buyer" ? "賣家" : "買家";

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post(`/api/orders/${order.id}/reviews`, { rating, comment });
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "送出失敗");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="space-y-3 rounded-xl bg-white p-4 shadow">
      <h2 className="font-bold">評價</h2>
      {order.my_review ? (
        <ReviewLine label={`我對${other}的評價`} review={order.my_review} />
      ) : (
        <form onSubmit={submit} className="space-y-2">
          <p className="text-sm text-gray-600">評價{other}（每筆訂單只能評價一次，送出後無法修改）</p>
          <div className="flex gap-4 text-sm">
            {(["POSITIVE", "NEGATIVE"] as const).map((r) => (
              <label key={r} className="flex items-center gap-1">
                <input type="radio" name="rating" checked={rating === r} onChange={() => setRating(r)} />
                {r === "POSITIVE" ? "👍 正評" : "👎 負評"}
              </label>
            ))}
          </div>
          <textarea
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            maxLength={500}
            placeholder="留言（選填，500 字內）"
            className="w-full rounded border px-3 py-2 text-sm"
            aria-label="評價留言"
          />
          <button disabled={busy} className="rounded bg-emerald-600 px-4 py-2 text-white disabled:opacity-50">
            送出評價
          </button>
          {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
        </form>
      )}
      {order.their_review && <ReviewLine label={`${other}對我的評價`} review={order.their_review} />}
    </section>
  );
}
