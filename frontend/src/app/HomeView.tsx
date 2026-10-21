"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { AuctionCard } from "@/components/AuctionCard";
import { api } from "@/lib/api";
import { SEARCHABLE_STATUSES, STATUS_LABELS, applyPrices, clockOffset } from "@/lib/auction";
import { usePolling, useServerNow } from "@/lib/hooks";
import type { AuctionList, AuctionSummary, PriceList } from "@/lib/types";

const PAGE_SIZE = 24;
const POLL_MS = 5000;
const MAX_POLL_IDS = 100;

export function HomeView() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const q = params.get("q") ?? "";
  const status = params.get("status") ?? "";

  const [items, setItems] = useState<AuctionSummary[]>([]);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [offset, setOffset] = useState(0);
  const [keyword, setKeyword] = useState(q);
  const now = useServerNow(offset);

  const load = useCallback(
    async (p: number) => {
      setLoading(true);
      setError(null);
      try {
        const qs = new URLSearchParams({ q, status, page: String(p) });
        const res = await api.get<AuctionList>(`/api/auctions?${qs}`);
        setOffset(clockOffset(res.server_time, Date.now()));
        setItems((prev) => (p === 1 ? res.items : [...prev, ...res.items]));
        setHasMore(res.items.length === PAGE_SIZE);
        setPage(p);
      } catch {
        setError("無法載入競標列表");
      } finally {
        setLoading(false);
      }
    },
    [q, status],
  );

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch on filter change
    void load(1);
  }, [load]);

  // One request refreshes every listed auction's price.
  usePolling(
    async () => {
      const ids = items.slice(0, MAX_POLL_IDS).map((it) => it.id);
      if (ids.length === 0) return;
      try {
        const res = await api.get<PriceList>(`/api/auctions/prices?ids=${ids.join(",")}`);
        setOffset(clockOffset(res.server_time, Date.now()));
        setItems((prev) => [...applyPrices(prev.slice(0, MAX_POLL_IDS), res.items), ...prev.slice(MAX_POLL_IDS)]);
      } catch {
        // transient polling failures are ignored; the next tick retries
      }
    },
    POLL_MS,
    items.length > 0,
  );

  function updateQuery(next: { q?: string; status?: string }) {
    const qs = new URLSearchParams();
    const nq = next.q ?? q;
    const ns = next.status ?? status;
    if (nq) qs.set("q", nq);
    if (ns) qs.set("status", ns);
    const s = qs.toString();
    router.replace(s ? `${pathname}?${s}` : pathname);
  }

  function onSearch(e: FormEvent) {
    e.preventDefault();
    updateQuery({ q: keyword.trim() });
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <form onSubmit={onSearch} className="flex flex-1 gap-2" role="search">
          <input
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            placeholder="搜尋商品名稱或描述"
            maxLength={50}
            className="min-w-0 flex-1 rounded border bg-white px-3 py-2"
            aria-label="關鍵字"
          />
          <button className="rounded bg-emerald-600 px-4 py-2 text-white hover:bg-emerald-700">搜尋</button>
        </form>
        <Link
          href="/auctions/new"
          className="rounded border border-emerald-600 px-4 py-2 text-center text-emerald-700 hover:bg-emerald-50"
        >
          ＋ 發布競標
        </Link>
      </div>

      <div className="flex flex-wrap gap-2" role="tablist" aria-label="狀態篩選">
        {["", ...SEARCHABLE_STATUSES].map((s) => (
          <button
            key={s || "ALL"}
            role="tab"
            aria-selected={status === s}
            onClick={() => updateQuery({ status: s })}
            className={`rounded-full px-3 py-1 text-sm ${status === s ? "bg-emerald-600 text-white" : "bg-white text-gray-700 shadow-sm"}`}
          >
            {s ? STATUS_LABELS[s as keyof typeof STATUS_LABELS] : "全部"}
          </button>
        ))}
      </div>

      {error && <p role="alert" className="text-red-600">{error}</p>}
      {!loading && items.length === 0 && !error && (
        <p className="rounded-xl bg-white p-8 text-center text-gray-500 shadow">目前沒有符合條件的競標</p>
      )}

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {items.map((a) => (
          <AuctionCard key={a.id} auction={a} now={now} />
        ))}
      </div>

      {hasMore && (
        <div className="text-center">
          <button
            onClick={() => void load(page + 1)}
            disabled={loading}
            className="rounded border bg-white px-4 py-2 text-sm hover:bg-gray-50 disabled:opacity-50"
          >
            {loading ? "載入中…" : "載入更多"}
          </button>
        </div>
      )}
    </div>
  );
}
