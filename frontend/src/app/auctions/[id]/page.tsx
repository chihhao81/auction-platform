"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { getAuction, getCurrentUser, placeBid, type Auction, type Bid } from "@/lib/auctions";

const money = new Intl.NumberFormat("zh-TW");

export default function AuctionDetailPage() {
  const params = useParams<{ id: string }>();
  const [auction, setAuction] = useState<Auction | null>(null);
  const [bids, setBids] = useState<Bid[]>([]);
  const [amount, setAmount] = useState(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [viewerID, setViewerID] = useState<number | null>(null);
  const refresh = useCallback(async () => {
    try { const result = await getAuction(params.id); setAuction(result.auction); setBids(result.bids); setAmount(result.auction.current_price + result.auction.min_increment); setError(""); try { const user = await getCurrentUser(); setViewerID(user.id); } catch { setViewerID(null); } }
    catch (cause) { setError(cause instanceof Error ? cause.message : "無法載入競標。"); }
  }, [params.id]);
  useEffect(() => { void refresh(); }, [refresh]);

  async function submitBid(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    try { await placeBid(params.id, amount); await refresh(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "出價失敗。"); }
    finally { setBusy(false); }
  }

  return <main className="featurePage"><header className="featureHeader"><Link href="/marketplace">← 返回市集</Link><span>小島競標</span></header>
    <section className="featureCard">
      {!auction ? <><h1>競標詳情</h1><p className="formError" role="alert">{error || "載入中…"}</p></> : <>
        <p className="sectionEyebrow">AUCTION DETAILS</p><h1>{auction.title}</h1><p className="featureLead">{auction.description}</p><p className="sellerDetail">賣家：{auction.seller_name}</p>
        {viewerID === auction.seller_id && auction.status === "UPCOMING" && <Link className="editAuctionLink" href={`/auctions/${auction.id}/edit`}>編輯競標資料</Link>}
        <div className="formGrid detailStats"><div><small>起標價格</small><strong>NT$ {money.format(auction.starting_price)}</strong></div><div><small>目前價格</small><strong>NT$ {money.format(auction.current_price)}</strong></div><div><small>出價次數</small><strong>{auction.bid_count}</strong></div><div><small>狀態</small><strong>{auction.status}</strong></div><div><small>開始時間</small><strong>{new Date(auction.starts_at).toLocaleString("zh-TW", { timeZone: "Asia/Taipei" })}</strong></div><div><small>結束時間</small><strong>{new Date(auction.ends_at).toLocaleString("zh-TW", { timeZone: "Asia/Taipei" })}</strong></div></div>
        <p>合法出價：NT$ {money.format(auction.current_price + auction.min_increment)}–{money.format(auction.current_price + auction.max_increment)}</p>
        <p>寄送方式：{auction.shipping_method || "賣家尚未填寫"}・運費：NT$ {money.format(auction.shipping_fee)}</p>
        {auction.status === "ACTIVE" && <form onSubmit={submitBid} className="bidForm"><label>你的出價（NT$）<input type="number" min={auction.current_price + auction.min_increment} max={auction.current_price + auction.max_increment} value={amount} onChange={(event) => setAmount(Number(event.target.value))} required /></label><button className="featureSubmit" disabled={busy}>{busy ? "出價中…" : "送出出價"}</button></form>}
        {auction.images.length > 0 && <div className="detailImages">{auction.images.map((image) => <img key={image} src={image} alt={auction.title} />)}</div>}
        <h2>最近出價</h2><ol className="bidHistory">{bids.map((bid) => <li key={bid.id}><span>{bid.bidder_name}</span><strong>NT$ {money.format(bid.amount)}</strong><time>{new Date(bid.created_at).toLocaleString("zh-TW", { timeZone: "Asia/Taipei" })}</time></li>)}{bids.length === 0 && <li>目前還沒有出價</li>}</ol>
      </>}
      {error && auction && <p className="formError" role="alert">{error}</p>}
    </section>
  </main>;
}
