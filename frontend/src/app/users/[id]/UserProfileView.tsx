"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ReviewList } from "@/components/ReviewList";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { reputationSummary } from "@/lib/reputation";
import { formatTaipei } from "@/lib/time";
import type { PublicProfile } from "@/lib/types";

export function UserProfileView() {
  const { id } = useParams<{ id: string }>();
  const { state } = useAuth();
  const [user, setUser] = useState<PublicProfile | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.get<{ user: PublicProfile }>(`/api/users/${id}`).then(
      (r) => setUser(r.user),
      (e) => setError(e instanceof ApiError && e.status === 404 ? "找不到此使用者" : "無法載入"),
    );
  }, [id]);

  if (error) return <p className="rounded-xl bg-white p-8 text-center text-gray-500 shadow">{error}</p>;
  if (!user) return <p className="text-center text-gray-500">載入中…</p>;

  const isMe = state.status === "authenticated" && state.me.user.id === user.id;
  const r = user.reputation;

  return (
    <div className="mx-auto max-w-2xl space-y-4">
      <section className="flex items-center gap-4 rounded-xl bg-white p-5 shadow">
        {user.avatar_url ? (
          // eslint-disable-next-line @next/next/no-img-element -- remote LINE avatar
          <img src={user.avatar_url} alt="" className="h-16 w-16 rounded-full" />
        ) : (
          <span className="flex h-16 w-16 items-center justify-center rounded-full bg-emerald-100 text-2xl">👤</span>
        )}
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-bold">
            {user.display_name}
            {user.banned && <span className="ml-2 rounded bg-red-100 px-1.5 text-sm text-red-700">已停權</span>}
          </h1>
          <p className="text-sm text-gray-500">加入時間 {formatTaipei(user.created_at)}</p>
        </div>
        {!isMe && state.status === "authenticated" && (
          <Link href={`/reports/new?type=USER&id=${user.id}`} className="text-sm text-gray-500 underline">
            檢舉
          </Link>
        )}
      </section>

      <section className="grid grid-cols-3 gap-3 text-center">
        <div className="rounded-xl bg-white p-3 shadow">
          <p className="text-2xl font-bold text-emerald-700">{r.positive}</p>
          <p className="text-xs text-gray-500">正評</p>
        </div>
        <div className="rounded-xl bg-white p-3 shadow">
          <p className="text-2xl font-bold text-red-600">{r.negative}</p>
          <p className="text-xs text-gray-500">負評</p>
        </div>
        <div className="rounded-xl bg-white p-3 shadow">
          <p className="text-2xl font-bold">{r.completed_orders}</p>
          <p className="text-xs text-gray-500">完成交易</p>
        </div>
      </section>

      <section className="rounded-xl bg-white p-4 shadow">
        <h2 className="mb-2 font-bold">
          被評價紀錄 <span className="text-sm font-normal text-gray-500">{reputationSummary(r) ?? ""}</span>
        </h2>
        <ReviewList reviews={user.reviews} />
      </section>
    </div>
  );
}
