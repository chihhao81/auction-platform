import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { memberRedirect } from "@/lib/auth";
import { LoginPanel, loginErrorMessage, safeNext } from "@/app/login/LoginPanel";
import type { Me } from "@/lib/types";

const me = (accepted: boolean): Me => ({
  user: { id: "u1", display_name: "A", avatar_url: "", role: "USER", status: "ACTIVE", created_at: "2026-01-01T00:00:00Z" },
  terms: { current_version: "v1", accepted },
});

describe("memberRedirect", () => {
  it("sends anonymous users to login with next", () => {
    expect(memberRedirect({ status: "anonymous", me: null }, "/me")).toBe("/login?next=%2Fme");
  });
  it("sends users without accepted terms to terms page", () => {
    expect(memberRedirect({ status: "authenticated", me: me(false) }, "/me")).toBe("/terms?next=%2Fme");
  });
  it("allows members and waits while loading", () => {
    expect(memberRedirect({ status: "authenticated", me: me(true) }, "/me")).toBeNull();
    expect(memberRedirect({ status: "loading", me: null }, "/me")).toBeNull();
  });
});

const searchParams = new URLSearchParams();
const replace = vi.fn();
const refresh = vi.fn().mockResolvedValue(undefined);
vi.mock("next/navigation", () => ({ useSearchParams: () => searchParams, useRouter: () => ({ replace }) }));
vi.mock("@/lib/auth", async (orig) => ({ ...(await orig<typeof import("@/lib/auth")>()), useAuth: () => ({ refresh }) }));

function stubMethods(methods: { line: boolean; mock: boolean; test: boolean }, onPost?: (body: string) => Response) {
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    if (url === "/api/auth/methods") return new Response(JSON.stringify(methods));
    return onPost ? onPost(String(init?.body)) : new Response(null, { status: 404 });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("LoginPanel", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    replace.mockReset();
    searchParams.delete("next");
    searchParams.delete("error");
  });

  it("maps error codes to messages and keeps next safe", () => {
    expect(loginErrorMessage("USER_BANNED")).toContain("停權");
    expect(loginErrorMessage("SOMETHING_ELSE")).toBe("登入失敗，請再試一次。");
    expect(loginErrorMessage(null)).toBeNull();
    expect(safeNext("//evil.com")).toBe("/");
    expect(safeNext("/auctions/1")).toBe("/auctions/1");
  });

  it("shows only the methods the backend enables", async () => {
    searchParams.set("next", "/auctions/1");
    searchParams.set("error", "USER_BANNED");
    stubMethods({ line: true, mock: false, test: false });
    render(<LoginPanel />);
    expect(await screen.findByRole("link", { name: "使用 LINE 登入" })).toHaveAttribute("href", "/api/auth/line/login?next=%2Fauctions%2F1");
    expect(screen.getByRole("alert")).toHaveTextContent("停權");
    expect(screen.queryByRole("button", { name: "測試登入" })).toBeNull();
  });

  it("signs in with the test login and follows next", async () => {
    document.cookie = "csrf_token=t";
    searchParams.set("next", "/me");
    const fetchMock = stubMethods({ line: false, mock: false, test: true }, () => new Response(null, { status: 204 }));
    render(<LoginPanel />);
    await userEvent.type(await screen.findByLabelText("測試帳號名稱"), "buyer1");
    await userEvent.type(screen.getByLabelText("測試密碼"), "secret-pass-123456");
    await userEvent.click(screen.getByRole("button", { name: "測試登入" }));
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/test-login", expect.objectContaining({
      method: "POST", body: JSON.stringify({ name: "buyer1", password: "secret-pass-123456" }),
    }));
    expect(refresh).toHaveBeenCalled();
    expect(replace).toHaveBeenCalledWith("/me");
    expect(screen.queryByRole("link", { name: "使用 LINE 登入" })).toBeNull();
  });

  it("shows the backend error for a wrong password", async () => {
    stubMethods({ line: false, mock: false, test: true }, () =>
      new Response(JSON.stringify({ error: { code: "INVALID_TEST_LOGIN", message: "測試帳號名稱或密碼錯誤" } }), { status: 401 }));
    render(<LoginPanel />);
    await userEvent.type(await screen.findByLabelText("測試帳號名稱"), "x");
    await userEvent.type(screen.getByLabelText("測試密碼"), "wrong");
    await userEvent.click(screen.getByRole("button", { name: "測試登入" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("密碼錯誤");
    expect(replace).not.toHaveBeenCalled();
  });
});
