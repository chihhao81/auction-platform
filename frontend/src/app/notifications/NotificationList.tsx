"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { NOTIFICATIONS_CHANGED } from "@/components/NotificationBell";
import { api } from "@/lib/api";
import { formatTaipei } from "@/lib/time";
import type { AppNotification } from "@/lib/types";

interface ListResponse {
  items: AppNotification[];
  has_more: boolean;
}

export function NotificationList() {
  const router = useRouter();
  const [items, setItems] = useState<AppNotification[] | null>(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);

  const load = useCallback(async (p: number) => {
    const res = await api.get<ListResponse>(`/api/notifications?page=${p}`);
    setItems((prev) => (p === 1 || !prev ? res.items : [...prev, ...res.items]));
    setHasMore(res.has_more);
    setPage(p);
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial fetch
    load(1).catch(() => setItems([]));
  }, [load]);

  const changed = () => window.dispatchEvent(new Event(NOTIFICATIONS_CHANGED));

  async function open(n: AppNotification) {
    if (!n.read_at) {
      await api.post(`/api/notifications/${n.id}/read`).catch(() => {});
      changed();
    }
    router.push(n.link);
  }

  async function readAll() {
    await api.post("/api/notifications/read-all");
    const now = new Date().toISOString();
    setItems((prev) => prev?.map((n) => ({ ...n, read_at: n.read_at ?? now })) ?? prev);
    changed();
  }

  const unread = items?.some((n) => !n.read_at) ?? false;

  return (
    <div className="mx-auto max-w-2xl space-y-3">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">通知</h1>
        <div className="flex gap-3 text-sm">
          <Link href="/me#notification-settings" className="text-gray-600 underline">
            通知設定
          </Link>
          {unread && (
            <button onClick={readAll} className="text-emerald-700 underline">
              全部標為已讀
            </button>
          )}
        </div>
      </div>
      {items === null ? (
        <p className="text-gray-500">載入中…</p>
      ) : items.length === 0 ? (
        <p className="rounded-xl bg-white p-8 text-center text-gray-500 shadow">目前沒有通知</p>
      ) : (
        <ul className="divide-y overflow-hidden rounded-xl bg-white shadow">
          {items.map((n) => (
            <li key={n.id}>
              <button
                onClick={() => void open(n)}
                className={`flex w-full gap-3 p-4 text-left hover:bg-gray-50 ${n.read_at ? "" : "bg-emerald-50/60"}`}
              >
                <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${n.read_at ? "bg-transparent" : "bg-emerald-600"}`} />
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{n.title}</span>
                  <span className="block break-words text-sm text-gray-600">{n.content}</span>
                  <span className="mt-1 block text-xs text-gray-400">{formatTaipei(n.created_at)}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {hasMore && (
        <div className="text-center">
          <button onClick={() => void load(page + 1)} className="rounded border bg-white px-4 py-2 text-sm">
            載入更多
          </button>
        </div>
      )}
    </div>
  );
}
