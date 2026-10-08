"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { createAuction, deleteAuctionImage, getAuction, updateAuction, uploadAuctionImage, type Auction, type AuctionInput } from "@/lib/auctions";

function nextHourLocal() {
  const date = new Date();
  date.setMinutes(0, 0, 0);
  date.setHours(date.getHours() + 1);
  return date;
}

function localInputDate(date: Date) {
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

export function AuctionCreateForm({ auctionId }: { auctionId?: string }) {
  const router = useRouter();
  const [startsAt, setStartsAt] = useState(() => nextHourLocal());
  const [endsAt, setEndsAt] = useState(() => new Date(nextHourLocal().getTime() + 48 * 60 * 60 * 1000));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [initial, setInitial] = useState<Auction | null>(null);
  const [loading, setLoading] = useState(Boolean(auctionId));
  const [existingImageIDs, setExistingImageIDs] = useState<string[]>([]);

  useEffect(() => {
    if (!auctionId) return;
    getAuction(auctionId).then(({ auction }) => {
      setInitial(auction);
      setExistingImageIDs(auction.images.map((url) => url.split("/").at(-1) ?? "" ).filter(Boolean));
      setStartsAt(new Date(auction.starts_at));
      setEndsAt(new Date(auction.ends_at));
    }).catch((cause) => setError(cause instanceof Error ? cause.message : "無法載入競標資料。"))
      .finally(() => setLoading(false));
  }, [auctionId]);

  async function submit(formData: FormData) {
    setBusy(true); setError(""); setSuccess("");
    const files = formData.getAll("images").filter((entry): entry is File => entry instanceof File && entry.size > 0);
    if (existingImageIDs.length + files.length > 4) { setError("一筆競標最多只能有 4 張圖片。"); setBusy(false); return; }
    const uploadedIDs: string[] = [];
    const oldImageIDs = existingImageIDs;
    try {
      for (const file of files) uploadedIDs.push((await uploadAuctionImage(file)).id);
      const input: AuctionInput = {
      title: String(formData.get("title") ?? "").trim(),
      description: String(formData.get("description") ?? "").trim(),
      starting_price: Number(formData.get("starting_price")),
      min_increment: Number(formData.get("min_increment")),
      max_increment: Number(formData.get("max_increment")),
      starts_at: new Date(startsAt).toISOString(),
      ends_at: new Date(endsAt).toISOString(),
      contact_method: String(formData.get("contact_method") ?? "").trim(),
      shipping_method: String(formData.get("shipping_method") ?? "").trim(),
      shipping_fee: Number(formData.get("shipping_fee")),
      images: [...existingImageIDs, ...uploadedIDs],
      };
      if (auctionId) {
        await updateAuction(auctionId, input);
        setSuccess("競標資料已更新。");
      } else {
        const result = await createAuction(input);
        setSuccess(`競標已建立（編號 ${result.id}）。`);
        router.push(`/auctions/${result.id}`);
      }
      setExistingImageIDs(input.images);
      if (auctionId) await Promise.all(oldImageIDs.filter((id) => !input.images.includes(id)).map((id) => deleteAuctionImage(id).catch(() => undefined)));
    } catch (cause) {
      await Promise.all(uploadedIDs.map((id) => deleteAuctionImage(id).catch(() => undefined)));
      setError(cause instanceof Error ? cause.message : "建立失敗，請稍後再試。");
    } finally { setBusy(false); }
  }

  return <main className="featurePage">
    <header className="featureHeader"><Link href="/marketplace">← 返回市集</Link><span>小島競標</span></header>
    <section className="featureCard">
      <p className="sectionEyebrow">{auctionId ? "EDIT AN AUCTION" : "CREATE AN AUCTION"}</p><h1>{auctionId ? "編輯競標商品" : "刊登競標商品"}</h1>
      <p className="featureLead">請提供清楚的商品與照護資訊。刊登功能需正式 LINE 登入，展示帳號無法送出資料。</p>
      {loading ? <p>載入競標資料中…</p> : <form key={initial?.id ?? "new-auction"} onSubmit={(event) => { event.preventDefault(); void submit(new FormData(event.currentTarget)); }} className="auctionForm">
        <label>商品名稱<input name="title" required maxLength={120} defaultValue={initial?.title ?? ""} placeholder="例如：橘夢豹紋守宮" /></label>
        <label>商品描述<textarea name="description" required maxLength={10000} rows={6} defaultValue={initial?.description ?? ""} placeholder="物種、性別、年齡、健康與照護資訊" /></label>
        <div className="formGrid"><label>起標價格（NT$）<input name="starting_price" type="number" min="0" defaultValue={initial?.starting_price ?? 0} required /></label><label>最小加價（NT$）<input name="min_increment" type="number" min="1" defaultValue={initial?.min_increment ?? 50} required /></label><label>最大加價（NT$）<input name="max_increment" type="number" min="1" defaultValue={initial?.max_increment ?? 50} required /></label><label>運費（NT$）<input name="shipping_fee" type="number" min="0" defaultValue={initial?.shipping_fee ?? 0} required /></label></div>
        <div className="formGrid"><label>開始時間<input type="datetime-local" value={localInputDate(startsAt)} onChange={(event) => setStartsAt(new Date(event.target.value))} required /></label><label>結束時間<input type="datetime-local" value={localInputDate(endsAt)} onChange={(event) => setEndsAt(new Date(event.target.value))} required /></label></div>
        <label>寄送方式<input name="shipping_method" maxLength={500} defaultValue={initial?.shipping_method ?? ""} placeholder="例如：超商店到店、郵寄" /></label>
        <label>額外聯絡方式<input name="contact_method" maxLength={500} defaultValue={initial?.contact_method ?? ""} placeholder="選填" /></label>
        <label>商品圖片（JPEG／PNG／GIF，單張最多 4 MB，最多 4 張）<input name="images" type="file" accept="image/jpeg,image/png,image/gif,.jpg,.jpeg,.png,.gif" multiple /></label>
        {initial?.images.length ? <div className="existingImages"><p>已上傳圖片（最多保留 4 張）</p>{initial.images.map((url) => { const id = url.split("/").at(-1) ?? ""; return <div key={id}><img src={url} alt="目前商品圖片" /><button type="button" onClick={() => { setExistingImageIDs((current) => current.filter((item) => item !== id)); setInitial((current) => current ? { ...current, images: current.images.filter((item) => item !== url) } : current); }}>移除</button></div>; })}</div> : null}
        {error && <p className="formError" role="alert">{error}</p>}{success && <p className="formSuccess" role="status">{success}</p>}
        <button className="featureSubmit" type="submit" disabled={busy}>{busy ? "送出中…" : auctionId ? "儲存修改" : "建立競標"}</button>
      </form>}
    </section>
  </main>;
}
