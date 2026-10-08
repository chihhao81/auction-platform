import { apiURL } from "./api";

export type AuctionInput = {
  title: string;
  description: string;
  starting_price: number;
  min_increment: number;
  max_increment: number;
  starts_at: string;
  ends_at: string;
  contact_method: string;
  shipping_method: string;
  shipping_fee: number;
  images: string[];
};

export type Auction = AuctionInput & {
  id: number;
  seller_id: number;
  seller_name: string;
  current_price: number;
  status: "UPCOMING" | "ACTIVE" | "ENDED" | "CLOSED";
  bid_count: number;
  highest_bidder_id?: number;
  created_at: string;
};

export type Bid = {
  id: number;
  bidder_id: number;
  bidder_name: string;
  amount: number;
  created_at: string;
};

type APIError = { error?: string; terms_version?: string };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiURL(path), {
    ...init,
    credentials: "include",
    headers: { ...(init?.body ? { "Content-Type": "application/json" } : {}), ...init?.headers },
  });
  const payload = (await response.json().catch(() => ({}))) as T & APIError;
  if (!response.ok) {
    if (response.status === 401) throw new Error("請先使用 LINE 登入；目前展示帳號尚未連接真實登入流程。")
    if (response.status === 428) throw new Error(`請先接受最新版使用者條款（${payload.terms_version ?? "最新版本"}）。`)
    throw new Error(payload.error ?? `API 請求失敗（${response.status}）`);
  }
  return payload;
}

export function createAuction(input: AuctionInput) {
  return request<{ id: number }>("/v1/auctions", { method: "POST", body: JSON.stringify(input) });
}

export function updateAuction(id: string, input: AuctionInput) {
  return request<{ auction: Auction; bids: Bid[] }>(`/v1/auctions/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify(input) });
}

export function getAuction(id: string) {
  return request<{ auction: Auction; bids: Bid[] }>(`/v1/auctions/${encodeURIComponent(id)}`);
}

export function placeBid(id: string, amount: number) {
  return request<{ auction_id: number; current_price: number; ends_at: string }>(`/v1/auctions/${encodeURIComponent(id)}/bids`, { method: "POST", body: JSON.stringify({ amount }) });
}

export function getCurrentUser() {
  return request<{ id: number; display_name: string }>("/v1/me");
}

export async function uploadAuctionImage(file: File): Promise<{ id: string; url: string }> {
  const form = new FormData();
  form.append("image", file);
  const response = await fetch(apiURL("/v1/uploads"), { method: "POST", credentials: "include", body: form });
  const payload = await response.json().catch(() => ({})) as { id?: string; url?: string; error?: string; terms_version?: string };
  if (!response.ok) {
    if (response.status === 401) throw new Error("請先使用 LINE 登入；目前展示帳號尚未連接真實登入流程。");
    if (response.status === 428) throw new Error(`請先接受最新版使用者條款（${payload.terms_version ?? "最新版本"}）。`);
    throw new Error(payload.error ?? `圖片上傳失敗（${response.status}）`);
  }
  if (!payload.id || !payload.url) throw new Error("圖片上傳回應格式錯誤。");
  return { id: payload.id, url: payload.url };
}

export async function deleteAuctionImage(id: string): Promise<void> {
  const response = await fetch(apiURL(`/v1/uploads/${encodeURIComponent(id)}`), { method: "DELETE", credentials: "include" });
  if (!response.ok) throw new Error("暫存圖片清理失敗。");
}
