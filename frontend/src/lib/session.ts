import { apiURL } from "./api";

export type User = { id: number; display_name: string; avatar_url: string };
export type Terms = { version: string; text: string };
export type SessionSnapshot =
  | { kind: "anonymous" }
  | { kind: "terms-required"; terms: Terms }
  | { kind: "signed-in"; user: User };

export async function readSession(fetcher: typeof fetch = fetch): Promise<SessionSnapshot> {
  const response = await fetcher(apiURL("/v1/me"), { credentials: "include", cache: "no-store" });
  if (response.status === 401) return { kind: "anonymous" };
  if (response.status === 428) {
    const termsResponse = await fetcher(apiURL("/v1/terms/current"), { credentials: "include", cache: "no-store" });
    if (!termsResponse.ok) throw new Error("目前尚未設定可供接受的使用者條款。");
    return { kind: "terms-required", terms: (await termsResponse.json()) as Terms };
  }
  if (!response.ok) throw new Error("無法確認登入狀態。");
  return { kind: "signed-in", user: (await response.json()) as User };
}

export async function acceptTerms(version: string, fetcher: typeof fetch = fetch): Promise<void> {
  const response = await fetcher(apiURL("/v1/terms/accept"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ version }),
  });
  if (!response.ok) throw new Error("條款接受紀錄未能儲存，請稍後重試。");
}

export async function logout(fetcher: typeof fetch = fetch): Promise<void> {
  const response = await fetcher(apiURL("/v1/logout"), { method: "POST", credentials: "include" });
  if (!response.ok) throw new Error("登出失敗。");
}
