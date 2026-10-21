"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { AuctionForm } from "@/components/AuctionForm";
import { api } from "@/lib/api";
import type { AuctionDetail, AuctionDetailResponse } from "@/lib/types";

export function EditAuctionView() {
  const { id } = useParams<{ id: string }>();
  const [auction, setAuction] = useState<AuctionDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.get<AuctionDetailResponse>(`/api/auctions/${id}`).then(
      (res) => setAuction(res.auction),
      () => setError("無法載入競標"),
    );
  }, [id]);

  if (error) return <p className="text-center text-gray-500">{error}</p>;
  if (!auction) return <p className="text-center text-gray-500">載入中…</p>;
  // UX guard only; the backend rejects edits by non-sellers or after start.
  if (!auction.is_seller) return <p className="text-center text-gray-500">只有賣家可以編輯此競標</p>;
  if (auction.status !== "UPCOMING") return <p className="text-center text-gray-500">競標已開始，無法再編輯</p>;
  return <AuctionForm existing={auction} />;
}
