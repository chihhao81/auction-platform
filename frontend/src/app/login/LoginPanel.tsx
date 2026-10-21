"use client";

import { useState } from "react";
import { useSearchParams } from "next/navigation";

const ERROR_MESSAGES: Record<string, string> = {
  USER_BANNED: "此帳號已被停權，無法登入。",
  INVALID_STATE: "登入逾時或驗證失敗，請重新登入。",
  LINE_DENIED: "您取消了 LINE 授權。",
  LINE_LOGIN_FAILED: "LINE 登入失敗，請稍後再試。",
  INVALID_MOCK_USER: "Mock 使用者名稱只能包含英數字、- 與 _。",
};

export function loginErrorMessage(code: string | null): string | null {
  if (!code) return null;
  return ERROR_MESSAGES[code] ?? "登入失敗，請再試一次。";
}

// Mock login is a development convenience; the backend refuses mock codes
// unless MOCK_LINE_AUTH=true, so exposing the form is harmless.
const MOCK_ENABLED = process.env.NEXT_PUBLIC_MOCK_LOGIN === "true";

export function LoginPanel() {
  const params = useSearchParams();
  const next = params.get("next") ?? "/";
  const error = loginErrorMessage(params.get("error"));
  const [mockUser, setMockUser] = useState("alice");

  const loginHref = `/api/auth/line/login?next=${encodeURIComponent(next)}`;

  return (
    <div className="mx-auto max-w-sm rounded-xl bg-white p-6 shadow">
      <h1 className="mb-4 text-xl font-bold">登入</h1>
      {error && <p role="alert" className="mb-4 rounded bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      <a
        href={loginHref}
        className="block rounded bg-[#06C755] px-4 py-2.5 text-center font-medium text-white hover:opacity-90"
      >
        使用 LINE 登入
      </a>
      {MOCK_ENABLED && (
        <form action="/api/auth/line/login" method="get" className="mt-6 border-t pt-4">
          <p className="mb-2 text-xs text-gray-500">開發模式：Mock 登入</p>
          <input type="hidden" name="next" value={next} />
          <div className="flex gap-2">
            <input
              name="mock_user"
              value={mockUser}
              onChange={(e) => setMockUser(e.target.value)}
              className="min-w-0 flex-1 rounded border px-2 py-1.5 text-sm"
              aria-label="Mock 使用者"
            />
            <button className="rounded bg-gray-700 px-3 py-1.5 text-sm text-white">Mock 登入</button>
          </div>
        </form>
      )}
    </div>
  );
}
