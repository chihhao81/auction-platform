import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api, readCookie } from "@/lib/api";

function jsonResponse(status: number, body: unknown) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("api client", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    document.cookie = "csrf_token=tok123";
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("sends the CSRF token on unsafe requests", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await api.post("/api/x", { a: 1 });
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers["X-CSRF-Token"]).toBe("tok123");
    expect(init.headers["Content-Type"]).toBe("application/json");
    expect(init.body).toBe('{"a":1}');
    expect(init.credentials).toBe("same-origin");
  });

  it("does not send the CSRF token on GET", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { ok: true }));
    await expect(api.get("/api/x")).resolves.toEqual({ ok: true });
    expect(fetchMock.mock.calls[0][1].headers["X-CSRF-Token"]).toBeUndefined();
  });

  it("maps backend errors to ApiError", async () => {
    fetchMock.mockResolvedValue(jsonResponse(403, { error: { code: "TERMS_NOT_ACCEPTED", message: "請先同意" } }));
    const err = await api.get("/api/x").catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 403, code: "TERMS_NOT_ACCEPTED", message: "請先同意" });
  });

  it("handles non-JSON error bodies", async () => {
    fetchMock.mockResolvedValue(new Response("bad gateway", { status: 502 }));
    await expect(api.get("/api/x")).rejects.toMatchObject({ status: 502, code: "UNKNOWN" });
  });
});

describe("readCookie", () => {
  it("parses cookie strings", () => {
    expect(readCookie("b", "a=1; b=x%20y; c=3")).toBe("x y");
    expect(readCookie("missing", "a=1")).toBeNull();
  });
});
