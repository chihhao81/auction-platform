"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { reputationSummary } from "@/lib/reputation";
import type { PublicProfile, Reputation } from "@/lib/types";

interface Props {
  seller: { id: string; display_name: string; avatar_url: string };
  auctionId: string;
  canReport: boolean;
}

// Seller info with reputation. Fetched once per seller, not on every
// auction poll.
export function SellerCard({ seller, auctionId, canReport }: Props) {
  const [rep, setRep] = useState<Reputation | null>(null);

  useEffect(() => {
    api.get<{ user: PublicProfile }>(`/api/users/${seller.id}`).then((r) => setRep(r.user.reputation), () => {});
  }, [seller.id]);

  return (
    <section className="flex items-center gap-3 rounded-xl bg-white p-4 shadow">
      {seller.avatar_url ? (
        // eslint-disable-next-line @next/next/no-img-element -- remote LINE avatar
        <img src={seller.avatar_url} alt="" className="h-10 w-10 rounded-full" />
      ) : (
        <span className="flex h-10 w-10 items-center justify-center rounded-full bg-emerald-100">👤</span>
      )}
      <div className="min-w-0 flex-1">
        <p className="text-xs text-gray-500">賣家</p>
        <Link href={`/users/${seller.id}`} className="font-medium hover:underline">
          {seller.display_name}
        </Link>
        {rep && (
          <p className="text-xs text-gray-500">
            {reputationSummary(rep) ?? "尚無評價"} · 完成交易 {rep.completed_orders} 筆
          </p>
        )}
      </div>
      {canReport && (
        <Link href={`/reports/new?type=AUCTION&id=${auctionId}`} className="text-xs text-gray-500 underline">
          檢舉此競標
        </Link>
      )}
    </section>
  );
}
