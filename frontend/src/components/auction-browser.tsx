"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { apiURL } from "@/lib/api";
import type { Auction } from "@/lib/auctions";

const money = new Intl.NumberFormat("zh-TW");

export function AuctionBrowser() {
  const [items, setItems] = useState<Auction[]>([]);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const refresh = useCallback(async () => {
    const params = new URLSearchParams();
    if (query.trim()) params.set("q", query.trim());
    if (status) params.set("status", status);
    try {
      const response = await fetch(apiURL(`/v1/auctions${params.size ? `?${params}` : ""}`), { credentials: "include", cache: "no-store" });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error ?? `讀取失敗（${response.status}）`);
      setItems(payload.auctions ?? []); setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "目前無法連線至競標服務。");
    }
  }, [query, status]);
  useEffect(() => { void refresh(); const timer = window.setInterval(() => void refresh(), 5_000); return () => window.clearInterval(timer); }, [refresh]);

  return <main className="featurePage"><header className="featureHeader"><Link href="/marketplace">← 展示市集</Link><span>真實競標列表</span><Link href="/marketplace/new">刊登商品</Link></header>
    <section className="featureCard"><p className="sectionEyebrow">LIVE AUCTIONS</p><h1>競標商品</h1><p className="featureLead">資料由 Backend 提供；出價、建立及編輯均由伺服器驗證。</p>
      <div className="browserControls"><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜尋商品" aria-label="搜尋商品" /><select value={status} onChange={(event) => setStatus(event.target.value)} aria-label="競標狀態"><option value="">全部狀態</option><option value="UPCOMING">即將開始</option><option value="ACTIVE">進行中</option><option value="ENDED">已結束</option></select></div>
      {error && <p className="formError" role="alert">{error}</p>}
      <div className="liveAuctionList">{items.map((auction) => <Link href={`/auctions/${auction.id}`} className="liveAuctionItem" key={auction.id}>{auction.images[0] && <img className="liveAuctionImage" src={auction.images[0]} alt="" /> }<div><strong>{auction.title}</strong><span>{auction.seller_name}・{auction.status}</span></div><div><strong>NT$ {money.format(auction.current_price)}</strong><span>{auction.bid_count} 次出價</span></div><time>{new Date(auction.ends_at).toLocaleString("zh-TW", { timeZone: "Asia/Taipei" })} 結束</time></Link>)}{!error && items.length === 0 && <p className="emptyLiveAuctions">目前沒有符合條件的競標商品。</p>}</div>
    </section>
  </main>;
}
