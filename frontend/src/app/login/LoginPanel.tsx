"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";

const ERROR_MESSAGES: Record<string, string> = {
  USER_BANNED: "此帳號已被停權，無法登入。",
  INVALID_STATE: "登入逾時或驗證失敗，請重新登入。",
  LINE_DENIED: "您取消了 LINE 授權。",
  LINE_LOGIN_FAILED: "LINE 登入失敗，請稍後再試。",
  LINE_NOT_CONFIGURED: "LINE 登入尚未開放。",
  INVALID_MOCK_USER: "Mock 使用者名稱只能包含英數字、- 與 _。",
};

export function loginErrorMessage(code: string | null): string | null {
  if (!code) return null;
  return ERROR_MESSAGES[code] ?? "登入失敗，請再試一次。";
}

export interface LoginMethods {
  line: boolean;
  mock: boolean;
  test: boolean;
}

// Only same-origin relative paths are followed after login.
export function safeNext(next: string | null): string {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/";
}

export function LoginPanel() {
  const params = useSearchParams();
  const router = useRouter();
  const { refresh } = useAuth();
  const next = safeNext(params.get("next"));
  const [error, setError] = useState(loginErrorMessage(params.get("error")));
  // The backend decides which sign-in options exist (no rebuild needed to
  // turn test login on or off).
  const [methods, setMethods] = useState<LoginMethods | null>(null);
  const [mockUser, setMockUser] = useState("alice");
  const [testName, setTestName] = useState("");
  const [testPassword, setTestPassword] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.get<LoginMethods>("/api/auth/methods").then(setMethods, () => setMethods({ line: true, mock: false, test: false }));
  }, []);

  async function testLogin(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post("/api/auth/test-login", { name: testName.trim(), password: testPassword });
      await refresh();
      router.replace(next);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "登入失敗");
      setBusy(false);
    }
  }

  const loginHref = `/api/auth/line/login?next=${encodeURIComponent(next)}`;
  const input = "min-w-0 rounded border px-2 py-1.5 text-sm";

  return (
    <div className="mx-auto max-w-sm space-y-4 rounded-xl bg-white p-6 shadow">
      <h1 className="text-xl font-bold">登入</h1>
      {error && <p role="alert" className="rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      {methods?.line && (
        <a href={loginHref} className="block rounded bg-[#06C755] px-4 py-2.5 text-center font-medium text-white hover:opacity-90">
          使用 LINE 登入
        </a>
      )}
      {methods?.test && (
        <form onSubmit={testLogin} className="space-y-2 border-t pt-4">
          <p className="text-xs text-gray-500">
            測試登入：輸入任意名稱（英數字、- 與 _）即可建立或登入該測試帳號，需要測試密碼。
          </p>
          <input value={testName} onChange={(e) => setTestName(e.target.value)} placeholder="名稱，例如 buyer1"
            maxLength={32} required className={`${input} w-full`} aria-label="測試帳號名稱" />
          <input type="password" value={testPassword} onChange={(e) => setTestPassword(e.target.value)}
            placeholder="測試密碼" required className={`${input} w-full`} aria-label="測試密碼" autoComplete="current-password" />
          <button disabled={busy} className="w-full rounded bg-gray-800 px-3 py-2 text-sm text-white disabled:opacity-50">
            {busy ? "登入中…" : "測試登入"}
          </button>
        </form>
      )}
      {methods?.mock && (
        <form action="/api/auth/line/login" method="get" className="border-t pt-4">
          <p className="mb-2 text-xs text-gray-500">開發模式：Mock 登入</p>
          <input type="hidden" name="next" value={next} />
          <div className="flex gap-2">
            <input name="mock_user" value={mockUser} onChange={(e) => setMockUser(e.target.value)}
              className={`${input} flex-1`} aria-label="Mock 使用者" />
            <button className="rounded bg-gray-700 px-3 py-1.5 text-sm text-white">Mock 登入</button>
          </div>
        </form>
      )}
      {methods && !methods.line && !methods.test && !methods.mock && (
        <p className="text-sm text-gray-500">目前沒有可用的登入方式。</p>
      )}
    </div>
  );
}
