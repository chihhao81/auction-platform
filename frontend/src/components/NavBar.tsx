"use client";

import Link from "next/link";
import { useAuth } from "@/lib/auth";

export function NavBar() {
  const { state } = useAuth();
  return (
    <header className="sticky top-0 z-10 border-b border-gray-200 bg-white/95 backdrop-blur">
      <nav className="mx-auto flex max-w-5xl items-center justify-between gap-3 px-4 py-3">
        <Link href="/" className="text-lg font-bold text-emerald-700">
          🦎 Critter Bid
        </Link>
        <div className="flex items-center gap-3 text-sm">
          {state.status === "authenticated" ? (
            <Link href="/me" className="flex items-center gap-2 hover:underline">
              {state.me.user.avatar_url ? (
                // eslint-disable-next-line @next/next/no-img-element -- remote LINE avatar, tiny
                <img src={state.me.user.avatar_url} alt="" className="h-7 w-7 rounded-full" />
              ) : (
                <span className="flex h-7 w-7 items-center justify-center rounded-full bg-emerald-100">👤</span>
              )}
              <span className="max-w-[8rem] truncate">{state.me.user.display_name}</span>
            </Link>
          ) : state.status === "anonymous" ? (
            <Link href="/login" className="rounded bg-emerald-600 px-3 py-1.5 font-medium text-white hover:bg-emerald-700">
              登入
            </Link>
          ) : null}
        </div>
      </nav>
    </header>
  );
}
