"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ApiError, api } from "@/lib/api";
import { formatPrice } from "@/lib/auction";
import { ORDER_STATUS_LABELS } from "@/lib/order";
import { formatTaipei } from "@/lib/time";
import type { OrderDetail } from "@/lib/types";
import { OrderActions } from "./OrderActions";
import { OrderMessages } from "./OrderMessages";
import { OrderReview } from "./OrderReview";

export function OrderDetailView() {
  const { id } = useParams<{ id: string }>();
  const [order, setOrder] = useState<OrderDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  const synced = useRef(false);

  const load = useCallback(async () => {
    try {
      const r = await api.get<{ order: OrderDetail }>(`/api/orders/${id}`);
      setOrder(r.order);
      // While awaiting payment, ask the backend to check ECPay once in case
      // its notification is delayed or lost (e.g. the buyer closed the tab).
      if (r.order.status === "AWAITING_PAYMENT" && !synced.current) {
        synced.current = true;
        await api.post(`/api/orders/${id}/payments/sync`).catch(() => {});
        const again = await api.get<{ order: OrderDetail }>(`/api/orders/${id}`);
        setOrder(again.order);
      }
    } catch (e) {
      setError(e instanceof ApiError && e.status === 404 ? "找不到此訂單" : "無法載入訂單");
    }
  }, [id]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial fetch
    void load();
  }, [load]);

  if (error) return <p className="rounded-xl bg-white p-8 text-center text-gray-500 shadow">{error}</p>;
  if (!order) return <p className="text-center text-gray-500">載入中…</p>;

  const counterparty = order.role === "buyer" ? order.seller : order.buyer;

  return (
    <div className="mx-auto max-w-2xl space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">訂單</h1>
        <span className="rounded bg-emerald-100 px-2 py-0.5 text-sm text-emerald-800">{ORDER_STATUS_LABELS[order.status]}</span>
      </div>

      {order.status === "AWAITING_CONTACT" && (
        <div className={`rounded-xl p-4 text-sm ${order.overdue ? "bg-red-50 text-red-800" : "bg-amber-50 text-amber-900"}`}>
          {order.overdue
            ? "買家未在期限內聯絡賣家，此訂單已標記逾期。"
            : order.role === "buyer"
              ? `請於 ${formatTaipei(order.contact_deadline)} 前透過訂單訊息聯絡賣家。`
              : `買家需於 ${formatTaipei(order.contact_deadline)} 前聯絡您。`}
        </div>
      )}

      <section className="rounded-xl bg-white p-4 shadow">
        <p className="text-sm text-gray-500">{order.role === "buyer" ? "賣家" : "買家"}</p>
        <p className="font-medium">{counterparty.display_name}</p>
        {order.role === "buyer" && order.seller_contact.length > 0 && (
          <div className="mt-2 text-sm">
            <p className="text-gray-500">賣家額外聯絡方式</p>
            {order.seller_contact.map((c) => (
              <p key={c} className="whitespace-pre-wrap break-words">
                {c}
              </p>
            ))}
          </div>
        )}
      </section>

      <section className="rounded-xl bg-white p-4 shadow">
        <ul className="divide-y">
          {order.items.map((it) => (
            <li key={it.auction_id} className="flex items-center gap-3 py-2">
              {it.image_url ? (
                // eslint-disable-next-line @next/next/no-img-element -- backend-resized image
                <img src={it.image_url} alt="" className="h-14 w-14 rounded object-cover" />
              ) : (
                <span className="flex h-14 w-14 items-center justify-center rounded bg-gray-100">🦎</span>
              )}
              <Link href={`/auctions/${it.auction_id}`} className="min-w-0 flex-1 truncate hover:underline">
                {it.title}
              </Link>
              <span className="font-medium">{formatPrice(it.amount)}</span>
            </li>
          ))}
        </ul>
        <dl className="mt-3 grid grid-cols-[1fr_auto] gap-y-1 border-t pt-3 text-sm">
          <dt className="text-gray-500">商品小計</dt>
          <dd>{formatPrice(order.items_total)}</dd>
          <dt className="text-gray-500">運費{order.items.length > 1 && "（合併寄送，取最高運費）"}</dt>
          <dd>{formatPrice(order.shipping_fee)}</dd>
          <dt className="font-bold">總金額</dt>
          <dd className="font-bold text-emerald-700">{formatPrice(order.total_amount)}</dd>
        </dl>
      </section>

      <OrderActions key={order.status} order={order} onChanged={() => void load()} />

      <OrderReview order={order} onChanged={() => void load()} />

      <OrderMessages orderId={order.id} disabled={order.status === "CANCELLED"} onSent={() => void load()} />

      <p className="text-center text-xs text-gray-400">訂單成立於 {formatTaipei(order.created_at)}</p>
    </div>
  );
}
