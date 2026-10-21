// Pure auction helpers used by the UI. They only drive UX hints; every rule
// is enforced again by the backend.
import type { AuctionPrice, AuctionStatus, AuctionSummary } from "./types";

export const STATUS_LABELS: Record<AuctionStatus, string> = {
  UPCOMING: "即將開始",
  ACTIVE: "進行中",
  ENDED: "已結束",
  CLOSED: "已關閉",
};

export const SEARCHABLE_STATUSES: AuctionStatus[] = ["ACTIVE", "UPCOMING", "ENDED"];

const priceFormat = new Intl.NumberFormat("zh-TW");

export function formatPrice(n: number): string {
  return `NT$ ${priceFormat.format(n)}`;
}

// Returns an error message for an invalid bid amount, or null if it is
// within the allowed range.
export function bidAmountError(raw: string, min: number, max: number): string | null {
  if (!/^\d+$/.test(raw.trim())) return "請輸入整數金額";
  const amount = Number(raw);
  if (amount < min || amount > max) return `出價需介於 ${priceFormat.format(min)} ~ ${priceFormat.format(max)}`;
  return null;
}

// Suggested quick-bid amounts between min and max (inclusive, deduplicated).
export function quickBids(min: number, max: number, count = 3): number[] {
  if (max <= min) return [min];
  const step = (max - min) / (count - 1);
  const out = new Set<number>();
  for (let i = 0; i < count; i++) out.add(Math.round(min + step * i));
  return [...out];
}

// Applies polled prices to the listed auctions. Auctions missing from the
// poll result have become CLOSED and are dropped from the home page.
export function applyPrices(items: AuctionSummary[], prices: AuctionPrice[]): AuctionSummary[] {
  const byId = new Map(prices.map((p) => [p.id, p]));
  return items.flatMap((it) => {
    const p = byId.get(it.id);
    if (!p) return [];
    return [{ ...it, current_price: p.current_price, bid_count: p.bid_count, end_time: p.end_time, status: p.status }];
  });
}

// The clock offset between server and browser, so countdowns are based on
// server time rather than a possibly wrong local clock.
export function clockOffset(serverTimeISO: string, localNowMs: number): number {
  const server = new Date(serverTimeISO).getTime();
  return Number.isNaN(server) ? 0 : server - localNowMs;
}
