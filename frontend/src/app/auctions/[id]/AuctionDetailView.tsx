"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { BidPanel } from "@/components/BidPanel";
import { ImageGallery } from "@/components/ImageGallery";
import { StatusBadge } from "@/components/StatusBadge";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { clockOffset, formatPrice } from "@/lib/auction";
import { usePolling, useServerNow } from "@/lib/hooks";
import { formatRemaining, formatTaipei } from "@/lib/time";
import type { AuctionDetail, AuctionDetailResponse } from "@/lib/types";

const POLL_MS = 5000;

export function AuctionDetailView() {
  const { id } = useParams<{ id: string }>();
  const { state } = useAuth();
  const [auction, setAuction] = useState<AuctionDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [offset, setOffset] = useState(0);
  const now = useServerNow(offset);

  const load = useCallback(async () => {
    try {
      const res = await api.get<AuctionDetailResponse>(`/api/auctions/${id}`);
      setOffset(clockOffset(res.server_time, Date.now()));
      setAuction(res.auction);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError && e.status === 404 ? "找不到此競標" : "無法載入競標");
    }
  }, [id]);

  // Reload when the login state changes so is_seller / is_highest_bidder update.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial fetch
    void load();
  }, [load, state.status]);

  const live = auction?.status === "ACTIVE" || auction?.status === "UPCOMING";
  usePolling(load, POLL_MS, live);

  if (error && !auction) return <p className="rounded-xl bg-white p-8 text-center text-gray-500 shadow">{error}</p>;
  if (!auction) return <p className="text-center text-gray-500">載入中…</p>;

  const upcoming = auction.status === "UPCOMING";
  const remaining = new Date(upcoming ? auction.start_time : auction.end_time).getTime() - now;
  const extended = auction.end_time !== auction.original_end_time;

  return (
    <div className="grid gap-6 md:grid-cols-2">
      <ImageGallery images={auction.images} alt={auction.title} />

      <div className="space-y-4">
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <StatusBadge status={auction.status} />
            {auction.is_seller && upcoming && (
              <Link href={`/auctions/${auction.id}/edit`} className="ml-auto text-sm text-emerald-700 underline">
                編輯
              </Link>
            )}
          </div>
          <h1 className="text-2xl font-bold break-words">{auction.title}</h1>
        </div>

        <section className="rounded-xl bg-white p-4 shadow">
          <p className="text-sm text-gray-500">目前價格</p>
          <p className="text-3xl font-bold text-emerald-700">{formatPrice(auction.current_price)}</p>
          <p className="mt-1 text-sm text-gray-500">
            {auction.bid_count} 次出價
            {(auction.status === "ACTIVE" || upcoming) && remaining > 0 && (
              <span className="ml-2 font-medium text-gray-800">
                {upcoming ? "距開始" : "剩餘"} {formatRemaining(remaining)}
              </span>
            )}
          </p>
          <div className="mt-4">
            <BidPanel auction={auction} loggedIn={state.status === "authenticated"} onBid={() => void load()} />
          </div>
        </section>

        <section className="rounded-xl bg-white p-4 text-sm shadow">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
            <dt className="text-gray-500">起標價格</dt>
            <dd>{formatPrice(auction.start_price)}</dd>
            <dt className="text-gray-500">加價範圍</dt>
            <dd>
              {formatPrice(auction.min_increment)} ~ {formatPrice(auction.max_increment)}
            </dd>
            <dt className="text-gray-500">開始時間</dt>
            <dd>{formatTaipei(auction.start_time)}</dd>
            <dt className="text-gray-500">結束時間</dt>
            <dd>
              {formatTaipei(auction.end_time)}
              {extended && <span className="ml-1 text-amber-700">（已延長，原定 {formatTaipei(auction.original_end_time)}）</span>}
            </dd>
            <dt className="text-gray-500">寄送方式</dt>
            <dd className="break-words">{auction.shipping_method}</dd>
            <dt className="text-gray-500">運費</dt>
            <dd>{formatPrice(auction.shipping_fee)}</dd>
            {auction.contact_info !== undefined && (
              <>
                <dt className="text-gray-500">額外聯絡方式</dt>
                <dd className="whitespace-pre-wrap break-words">{auction.contact_info || "（未填寫）"}</dd>
              </>
            )}
          </dl>
        </section>

        <section className="flex items-center gap-3 rounded-xl bg-white p-4 shadow">
          {auction.seller.avatar_url ? (
            // eslint-disable-next-line @next/next/no-img-element -- remote LINE avatar
            <img src={auction.seller.avatar_url} alt="" className="h-10 w-10 rounded-full" />
          ) : (
            <span className="flex h-10 w-10 items-center justify-center rounded-full bg-emerald-100">👤</span>
          )}
          <div>
            <p className="text-xs text-gray-500">賣家</p>
            <p className="font-medium">{auction.seller.display_name}</p>
          </div>
        </section>
      </div>

      <section className="rounded-xl bg-white p-4 shadow md:col-span-2">
        <h2 className="mb-2 font-bold">商品描述</h2>
        {/* Plain text: React escapes the content, so no HTML is rendered. */}
        <p className="whitespace-pre-wrap break-words text-sm leading-7">{auction.description || "（無描述）"}</p>
      </section>

      <section className="rounded-xl bg-white p-4 shadow md:col-span-2">
        <h2 className="mb-2 font-bold">出價紀錄</h2>
        {auction.bids.length === 0 ? (
          <p className="text-sm text-gray-500">尚無出價</p>
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-gray-500">
              <tr>
                <th className="py-1 font-normal">出價者</th>
                <th className="py-1 font-normal">金額</th>
                <th className="py-1 text-right font-normal">時間</th>
              </tr>
            </thead>
            <tbody>
              {auction.bids.map((b) => (
                <tr key={b.id} className="border-t">
                  <td className="py-1.5">
                    {b.bidder_name}
                    {b.is_mine && <span className="ml-1 text-emerald-700">（我）</span>}
                  </td>
                  <td className="py-1.5 font-medium">{formatPrice(b.amount)}</td>
                  <td className="py-1.5 text-right text-gray-500">{formatTaipei(b.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
