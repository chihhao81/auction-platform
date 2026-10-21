import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { memberRedirect } from "@/lib/auth";
import { LoginPanel, loginErrorMessage } from "@/app/login/LoginPanel";
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
vi.mock("next/navigation", () => ({ useSearchParams: () => searchParams }));

describe("LoginPanel", () => {
  it("maps error codes to messages", () => {
    expect(loginErrorMessage("USER_BANNED")).toContain("停權");
    expect(loginErrorMessage("SOMETHING_ELSE")).toBe("登入失敗，請再試一次。");
    expect(loginErrorMessage(null)).toBeNull();
  });

  it("links to backend login preserving next and shows errors", () => {
    searchParams.set("next", "/auctions/1");
    searchParams.set("error", "USER_BANNED");
    render(<LoginPanel />);
    expect(screen.getByRole("link", { name: "使用 LINE 登入" })).toHaveAttribute(
      "href",
      "/api/auth/line/login?next=%2Fauctions%2F1",
    );
    expect(screen.getByRole("alert")).toHaveTextContent("停權");
  });
});
