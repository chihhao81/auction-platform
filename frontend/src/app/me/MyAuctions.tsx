"use client";

import { useEffect, useState } from "react";
import { AuctionCard } from "@/components/AuctionCard";
import { api } from "@/lib/api";
import { STATUS_LABELS, clockOffset } from "@/lib/auction";
import { useServerNow } from "@/lib/hooks";
import type { AuctionList, AuctionSummary } from "@/lib/types";

const ROLES = [
  { key: "selling", label: "我賣的" },
  { key: "bidding", label: "我出價的" },
] as const;
const STATUSES = ["UPCOMING", "ACTIVE", "ENDED"] as const;

export function MyAuctions() {
  const [role, setRole] = useState<(typeof ROLES)[number]["key"]>("selling");
  const [status, setStatus] = useState<(typeof STATUSES)[number]>("ACTIVE");
  const [items, setItems] = useState<AuctionSummary[] | null>(null);
  const [offset, setOffset] = useState(0);
  const now = useServerNow(offset);

  useEffect(() => {
    let cancelled = false;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset while loading
    setItems(null);
    api.get<AuctionList>(`/api/me/auctions?role=${role}&status=${status}`).then(
      (res) => {
        if (cancelled) return;
        setOffset(clockOffset(res.server_time, Date.now()));
        setItems(res.items);
      },
      () => !cancelled && setItems([]),
    );
    return () => {
      cancelled = true;
    };
  }, [role, status]);

  const tab = (active: boolean) =>
    `rounded-full px-3 py-1 text-sm ${active ? "bg-emerald-600 text-white" : "bg-gray-100 text-gray-700"}`;

  return (
    <section className="space-y-3 rounded-xl bg-white p-5 shadow">
      <h2 className="text-lg font-bold">我的競標</h2>
      <div className="flex flex-wrap gap-2">
        {ROLES.map((r) => (
          <button key={r.key} className={tab(role === r.key)} onClick={() => setRole(r.key)}>
            {r.label}
          </button>
        ))}
        <span className="mx-1 border-l" />
        {STATUSES.map((s) => (
          <button key={s} className={tab(status === s)} onClick={() => setStatus(s)}>
            {STATUS_LABELS[s]}
          </button>
        ))}
      </div>
      {items === null ? (
        <p className="text-sm text-gray-500">載入中…</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-gray-500">沒有競標</p>
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {items.map((a) => (
            <AuctionCard key={a.id} auction={a} now={now} />
          ))}
        </div>
      )}
    </section>
  );
}
