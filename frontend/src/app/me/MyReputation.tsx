"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ReviewList } from "@/components/ReviewList";
import { api } from "@/lib/api";
import { REPORT_STATUS_LABELS, reputationSummary } from "@/lib/reputation";
import { formatTaipei } from "@/lib/time";
import type { PublicProfile, Report } from "@/lib/types";

export function MyReputation({ userId }: { userId: string }) {
  const [profile, setProfile] = useState<PublicProfile | null>(null);

  useEffect(() => {
    api.get<{ user: PublicProfile }>(`/api/users/${userId}`).then((r) => setProfile(r.user), () => {});
  }, [userId]);

  if (!profile) return null;
  const r = profile.reputation;
  return (
    <section className="space-y-3 rounded-xl bg-white p-5 shadow">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-bold">我的評價</h2>
        <Link href={`/users/${userId}`} className="text-sm text-emerald-700 underline">
          查看公開頁面
        </Link>
      </div>
      <p className="text-sm">
        👍 {r.positive}　👎 {r.negative}　完成交易 {r.completed_orders} 筆
        {reputationSummary(r) && <span className="ml-2 text-gray-500">{reputationSummary(r)}</span>}
      </p>
      <ReviewList reviews={profile.reviews} />
    </section>
  );
}

export function MyReports() {
  const [items, setItems] = useState<Report[] | null>(null);

  useEffect(() => {
    api.get<{ items: Report[] }>("/api/me/reports").then((r) => setItems(r.items), () => setItems([]));
  }, []);

  return (
    <section id="my-reports" className="space-y-3 rounded-xl bg-white p-5 shadow">
      <h2 className="text-lg font-bold">檢舉紀錄</h2>
      {items === null ? (
        <p className="text-sm text-gray-500">載入中…</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-gray-500">沒有檢舉紀錄</p>
      ) : (
        <ul className="divide-y text-sm">
          {items.map((r) => (
            <li key={r.id} className="py-2">
              <div className="flex items-center gap-2">
                <span>{r.target_type === "AUCTION" ? "競標" : "使用者"}</span>
                <span className="text-gray-600">{r.reason_label}</span>
                <span className="ml-auto rounded bg-gray-100 px-1.5 text-xs">{REPORT_STATUS_LABELS[r.status]}</span>
              </div>
              <p className="text-xs text-gray-400">{formatTaipei(r.created_at)}</p>
              {r.resolution_note && <p className="mt-1 text-gray-600">處理說明：{r.resolution_note}</p>}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
