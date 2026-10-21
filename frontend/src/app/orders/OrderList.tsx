"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { formatPrice } from "@/lib/auction";
import { ORDER_STATUS_LABELS } from "@/lib/order";
import { formatTaipei } from "@/lib/time";
import type { OrderSummary } from "@/lib/types";

const ROLES = [
  { key: "buyer", label: "我買的" },
  { key: "seller", label: "我賣的" },
] as const;

export function OrderList() {
  const [role, setRole] = useState<"buyer" | "seller">("buyer");
  const [items, setItems] = useState<OrderSummary[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset while loading
    setItems(null);
    api.get<{ items: OrderSummary[] }>(`/api/orders?role=${role}`).then(
      (r) => !cancelled && setItems(r.items),
      () => !cancelled && setItems([]),
    );
    return () => {
      cancelled = true;
    };
  }, [role]);

  return (
    <div className="mx-auto max-w-2xl space-y-3">
      <h1 className="text-xl font-bold">我的訂單</h1>
      <div className="flex gap-2">
        {ROLES.map((r) => (
          <button
            key={r.key}
            onClick={() => setRole(r.key)}
            className={`rounded-full px-3 py-1 text-sm ${role === r.key ? "bg-emerald-600 text-white" : "bg-white shadow-sm"}`}
          >
            {r.label}
          </button>
        ))}
      </div>
      {items === null ? (
        <p className="text-gray-500">載入中…</p>
      ) : items.length === 0 ? (
        <p className="rounded-xl bg-white p-8 text-center text-gray-500 shadow">沒有訂單</p>
      ) : (
        <ul className="divide-y overflow-hidden rounded-xl bg-white shadow">
          {items.map((o) => (
            <li key={o.id}>
              <Link href={`/orders/${o.id}`} className="flex items-center gap-3 p-4 hover:bg-gray-50">
                <div className="min-w-0 flex-1">
                  <p className="font-medium">
                    {role === "buyer" ? "賣家" : "買家"}：{o.counterparty_name}
                  </p>
                  <p className="text-sm text-gray-500">
                    {o.item_count} 件 · {formatTaipei(o.created_at)}
                  </p>
                </div>
                <div className="text-right">
                  <p className="font-bold">{formatPrice(o.total_amount)}</p>
                  <p className="text-xs">
                    {ORDER_STATUS_LABELS[o.status]}
                    {o.overdue && <span className="ml-1 text-red-600">逾期</span>}
                  </p>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
