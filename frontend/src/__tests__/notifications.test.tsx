import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { formatBadge } from "@/components/NotificationBell";
import { urlBase64ToUint8Array } from "@/lib/push";
import { NotificationList } from "@/app/notifications/NotificationList";
import type { AppNotification } from "@/lib/types";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

describe("formatBadge", () => {
  it("hides zero and caps large counts", () => {
    expect(formatBadge(0)).toBeNull();
    expect(formatBadge(7)).toBe("7");
    expect(formatBadge(150)).toBe("99+");
  });
});

describe("urlBase64ToUint8Array", () => {
  it("decodes unpadded base64url", () => {
    // bytes 0xfb 0xff 0xbf encode to "-_-_" in base64url
    expect(Array.from(urlBase64ToUint8Array("-_-_"))).toEqual([0xfb, 0xff, 0xbf]);
    expect(Array.from(urlBase64ToUint8Array("AQI"))).toEqual([1, 2]);
  });
});

const notif = (over: Partial<AppNotification>): AppNotification => ({
  id: "n1",
  type: "OUTBID",
  title: "您的出價已被超越",
  content: "「守宮」目前價格 NT$ 1,100。",
  related_auction_id: "a1",
  related_order_id: null,
  link: "/auctions/a1",
  read_at: null,
  created_at: "2026-10-21T12:00:00Z",
  ...over,
});

describe("NotificationList", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    push.mockReset();
  });

  it("marks an unread notification read and navigates to its link", async () => {
    document.cookie = "csrf_token=t";
    const fetchMock = vi.fn(async (url: string) => {
      if (url.startsWith("/api/notifications?page=1")) {
        return new Response(JSON.stringify({ items: [notif({}), notif({ id: "n2", read_at: "2026-10-21T12:01:00Z", title: "舊通知" })], has_more: false }));
      }
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal("fetch", fetchMock);
    const changed = vi.fn();
    window.addEventListener("notifications:changed", changed);

    render(<NotificationList />);
    await userEvent.click(await screen.findByText("您的出價已被超越"));

    expect(fetchMock).toHaveBeenCalledWith("/api/notifications/n1/read", expect.objectContaining({ method: "POST" }));
    expect(changed).toHaveBeenCalled();
    expect(push).toHaveBeenCalledWith("/auctions/a1");
    expect(screen.getByRole("button", { name: "全部標為已讀" })).toBeInTheDocument();
    window.removeEventListener("notifications:changed", changed);
  });

  it("does not re-mark already read notifications", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ items: [notif({ read_at: "2026-10-21T12:01:00Z" })], has_more: false })),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<NotificationList />);
    await userEvent.click(await screen.findByText("您的出價已被超越"));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: "全部標為已讀" })).toBeNull();
  });
});
