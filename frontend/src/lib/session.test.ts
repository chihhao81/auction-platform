import { describe, expect, it, vi } from "vitest";
import { acceptTerms, logout, readSession } from "./session";

describe("readSession", () => {
  it("returns anonymous when the backend has no session", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 401 }));
    await expect(readSession(fetcher)).resolves.toEqual({ kind: "anonymous" });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("loads the current terms when acceptance is required", async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 428 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ version: "v1", text: "條款" }), { status: 200 }));
    await expect(readSession(fetcher)).resolves.toEqual({ kind: "terms-required", terms: { version: "v1", text: "條款" } });
    expect(fetcher).toHaveBeenNthCalledWith(2, expect.stringContaining("/v1/terms/current"), expect.objectContaining({ credentials: "include" }));
  });

  it("returns the logged in user", async () => {
    const user = { id: 12, display_name: "島民", avatar_url: "" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify(user), { status: 200 }));
    await expect(readSession(fetcher)).resolves.toEqual({ kind: "signed-in", user });
  });

  it("surfaces unavailable terms instead of allowing acceptance", async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 428 }))
      .mockResolvedValueOnce(new Response(null, { status: 503 }));
    await expect(readSession(fetcher)).rejects.toThrow("目前尚未設定可供接受的使用者條款");
  });
});

describe("terms and logout actions", () => {
  it("sends the exact terms version as a credentialed POST", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 204 }));
    await expect(acceptTerms("v1", fetcher)).resolves.toBeUndefined();
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/v1/terms/accept"), expect.objectContaining({
      method: "POST", credentials: "include", body: JSON.stringify({ version: "v1" }),
    }));
  });

  it("rejects a failed terms acceptance", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 500 }));
    await expect(acceptTerms("v1", fetcher)).rejects.toThrow("條款接受紀錄未能儲存");
  });

  it("clears the session using a credentialed POST", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 204 }));
    await expect(logout(fetcher)).resolves.toBeUndefined();
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/v1/logout"), expect.objectContaining({ method: "POST", credentials: "include" }));
  });
});
