"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { usePolling } from "@/lib/hooks";

// Fired by pages that change read state so the badge refreshes immediately.
export const NOTIFICATIONS_CHANGED = "notifications:changed";

const POLL_MS = 30_000;

export function formatBadge(count: number): string | null {
  if (count <= 0) return null;
  return count > 99 ? "99+" : String(count);
}

export function NotificationBell() {
  const [count, setCount] = useState(0);

  const refresh = useCallback(async () => {
    try {
      const res = await api.get<{ count: number }>("/api/notifications/unread-count");
      setCount(res.count);
    } catch {
      // ignored; next poll retries
    }
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial load
    void refresh();
    const onChange = () => void refresh();
    window.addEventListener(NOTIFICATIONS_CHANGED, onChange);
    return () => window.removeEventListener(NOTIFICATIONS_CHANGED, onChange);
  }, [refresh]);

  usePolling(refresh, POLL_MS);

  const badge = formatBadge(count);
  return (
    <Link href="/notifications" className="relative p-1 text-xl" aria-label={badge ? `通知（${badge} 則未讀）` : "通知"}>
      🔔
      {badge && (
        <span className="absolute -right-1 -top-1 min-w-5 rounded-full bg-red-600 px-1 text-center text-xs font-bold leading-5 text-white">
          {badge}
        </span>
      )}
    </Link>
  );
}
