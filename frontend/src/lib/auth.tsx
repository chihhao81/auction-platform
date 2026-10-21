"use client";

import { Suspense, createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { ApiError, api } from "./api";
import { disablePush } from "./push";
import type { Me } from "./types";

type AuthState =
  | { status: "loading"; me: null }
  | { status: "anonymous"; me: null }
  | { status: "authenticated"; me: Me };

interface AuthContextValue {
  state: AuthState;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading", me: null });

  const refresh = useCallback(async () => {
    try {
      const me = await api.get<Me>("/api/me");
      setState({ status: "authenticated", me });
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setState({ status: "anonymous", me: null });
        return;
      }
      throw e;
    }
  }, []);

  const logout = useCallback(async () => {
    // Stop this browser receiving the account's pushes (shared devices).
    await disablePush().catch(() => {});
    try {
      await api.post("/api/auth/logout");
    } catch (e) {
      // An already expired session is effectively logged out.
      if (!(e instanceof ApiError && e.status === 401)) throw e;
    }
    setState({ status: "anonymous", me: null });
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial session load
    refresh().catch(() => setState({ status: "anonymous", me: null }));
  }, [refresh]);

  return <AuthContext.Provider value={{ state, refresh, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}

// Decides where a visitor must go before seeing a member-only page.
// Returns null when access is allowed. Exported for unit tests.
export function memberRedirect(state: AuthState, pathname: string): string | null {
  const next = encodeURIComponent(pathname);
  if (state.status === "anonymous") return `/login?next=${next}`;
  if (state.status === "authenticated" && !state.me.terms.accepted) return `/terms?next=${next}`;
  return null;
}

// Renders children only for logged-in users who accepted the current terms.
// This is UX only — the backend enforces the same rules on every request.
// The Suspense boundary is required because usePathname() is dynamic on
// dynamic routes and cannot run during prerendering.
export function RequireMember({ children }: { children: ReactNode }) {
  return (
    <Suspense fallback={<p className="p-6 text-center text-gray-500">載入中…</p>}>
      <MemberGate>{children}</MemberGate>
    </Suspense>
  );
}

function MemberGate({ children }: { children: ReactNode }) {
  const { state } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const target = memberRedirect(state, pathname);

  useEffect(() => {
    if (target) router.replace(target);
  }, [target, router]);

  if (state.status !== "authenticated" || target) {
    return <p className="p-6 text-center text-gray-500">載入中…</p>;
  }
  return <>{children}</>;
}
