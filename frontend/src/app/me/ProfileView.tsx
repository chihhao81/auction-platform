"use client";

import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { formatTaipei } from "@/lib/time";
import { MyAuctions } from "./MyAuctions";
import { NotificationSettings } from "./NotificationSettings";

export function ProfileView() {
  const { state, logout } = useAuth();
  const router = useRouter();
  if (state.status !== "authenticated") return null;
  const { user } = state.me;

  async function onLogout() {
    await logout();
    router.replace("/");
  }

  return (
    <div className="space-y-6">
      <section className="flex items-center gap-4 rounded-xl bg-white p-5 shadow">
        {user.avatar_url ? (
          // eslint-disable-next-line @next/next/no-img-element -- remote LINE avatar
          <img src={user.avatar_url} alt="" className="h-16 w-16 rounded-full" />
        ) : (
          <span className="flex h-16 w-16 items-center justify-center rounded-full bg-emerald-100 text-2xl">👤</span>
        )}
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-bold">{user.display_name}</h1>
          <p className="text-sm text-gray-500">
            加入時間 {formatTaipei(user.created_at)}
            {user.role === "ADMIN" && <span className="ml-2 rounded bg-amber-100 px-1.5 text-amber-800">管理員</span>}
          </p>
        </div>
        <button onClick={onLogout} className="rounded border px-3 py-1.5 text-sm hover:bg-gray-50">
          登出
        </button>
      </section>
      <MyAuctions />
      <NotificationSettings />
    </div>
  );
}
