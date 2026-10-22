"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ApiError, apiRequest } from "@/lib/api";
import { shrinkImage } from "@/lib/imageResize";
import { isoToTaipeiLocalInput, taipeiLocalInputToISO } from "@/lib/time";
import type { AuctionDetail, AuctionImage } from "@/lib/types";

const MAX_IMAGES = 4;
const ACCEPT = "image/jpeg,image/png,image/webp";

export interface FormValues {
  title: string;
  description: string;
  start_price: string;
  min_increment: string;
  max_increment: string;
  start_time: string; // datetime-local, Asia/Taipei
  end_time: string;
  contact_info: string;
  shipping_method: string;
  shipping_fee: string;
}

// Defaults from the spec: start at the next full hour, run for 48 hours,
// start price 0, increments 50 ~ 50.
export function defaultValues(nowMs: number): FormValues {
  const start = new Date(Math.floor(nowMs / 3600_000) * 3600_000 + 3600_000);
  const end = new Date(start.getTime() + 48 * 3600_000);
  return {
    title: "",
    description: "",
    start_price: "0",
    min_increment: "50",
    max_increment: "50",
    start_time: isoToTaipeiLocalInput(start.toISOString()),
    end_time: isoToTaipeiLocalInput(end.toISOString()),
    contact_info: "",
    shipping_method: "",
    shipping_fee: "0",
  };
}

export function valuesFromAuction(a: AuctionDetail): FormValues {
  return {
    title: a.title,
    description: a.description,
    start_price: String(a.start_price),
    min_increment: String(a.min_increment),
    max_increment: String(a.max_increment),
    start_time: isoToTaipeiLocalInput(a.start_time),
    end_time: isoToTaipeiLocalInput(a.end_time),
    contact_info: a.contact_info ?? "",
    shipping_method: a.shipping_method,
    shipping_fee: String(a.shipping_fee),
  };
}

// Client-side checks for quick feedback; the backend re-validates all of it.
export function validateForm(v: FormValues, imageCount: number): string | null {
  if (!v.title.trim()) return "請填寫商品名稱";
  if (!v.shipping_method.trim()) return "請填寫寄送方式";
  for (const [k, label] of [
    ["start_price", "起標價格"],
    ["min_increment", "最小加價"],
    ["max_increment", "最高加價"],
    ["shipping_fee", "運費"],
  ] as const) {
    if (!/^\d+$/.test(v[k])) return `${label}需為非負整數`;
  }
  if (Number(v.min_increment) < 1) return "最小加價至少為 1";
  if (Number(v.max_increment) < Number(v.min_increment)) return "最高加價不可小於最小加價";
  if (!v.start_time || !v.end_time) return "請填寫開始與結束時間";
  if (v.end_time <= v.start_time) return "結束時間需晚於開始時間";
  if (imageCount < 1 || imageCount > MAX_IMAGES) return `請上傳 1 ~ ${MAX_IMAGES} 張圖片`;
  return null;
}

export function toPayload(v: FormValues) {
  return {
    title: v.title,
    description: v.description,
    start_price: Number(v.start_price),
    min_increment: Number(v.min_increment),
    max_increment: Number(v.max_increment),
    start_time: taipeiLocalInputToISO(v.start_time),
    end_time: taipeiLocalInputToISO(v.end_time),
    contact_info: v.contact_info,
    shipping_method: v.shipping_method,
    shipping_fee: Number(v.shipping_fee),
  };
}

interface Props {
  existing?: AuctionDetail;
}

export function AuctionForm({ existing }: Props) {
  const router = useRouter();
  const [values, setValues] = useState<FormValues>(() =>
    existing ? valuesFromAuction(existing) : defaultValues(Date.now()),
  );
  const [kept, setKept] = useState<AuctionImage[]>(existing?.images ?? []);
  const [files, setFiles] = useState<File[]>([]);
  const [previews, setPreviews] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const urls = files.map((f) => URL.createObjectURL(f));
    // eslint-disable-next-line react-hooks/set-state-in-effect -- object URLs follow the file list
    setPreviews(urls);
    return () => urls.forEach((u) => URL.revokeObjectURL(u));
  }, [files]);

  const set = (k: keyof FormValues) => (e: { target: { value: string } }) =>
    setValues((v) => ({ ...v, [k]: e.target.value }));

  function addFiles(list: FileList | null) {
    // Copy now: the FileList is live and is emptied as soon as the input is
    // reset (e.target.value = ""), which happens before the state updater runs.
    const picked = list ? Array.from(list) : [];
    if (picked.length === 0) return;
    setFiles((prev) => [...prev, ...picked].slice(0, Math.max(0, MAX_IMAGES - kept.length)));
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const err = validateForm(values, kept.length + files.length);
    if (err) {
      setError(err);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const fd = new FormData();
      const payload = existing ? { ...toPayload(values), keep_image_ids: kept.map((k) => k.id) } : toPayload(values);
      fd.set("data", JSON.stringify(payload));
      for (const f of files) fd.append("images", await shrinkImage(f));
      const res = await apiRequest<{ id: string }>(existing ? `/api/auctions/${existing.id}` : "/api/auctions", {
        method: existing ? "PUT" : "POST",
        body: fd,
      });
      router.push(`/auctions/${res.id}`);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "儲存失敗，請稍後再試");
      setSubmitting(false);
    }
  }

  const input = "w-full rounded border px-3 py-2";
  const imageCount = kept.length + files.length;

  return (
    <form onSubmit={submit} className="mx-auto max-w-2xl space-y-4 rounded-xl bg-white p-5 shadow">
      <h1 className="text-xl font-bold">{existing ? "編輯競標" : "發布競標"}</h1>

      <label className="block space-y-1">
        <span className="text-sm font-medium">商品名稱 *</span>
        <input className={input} value={values.title} onChange={set("title")} maxLength={100} required />
      </label>

      <label className="block space-y-1">
        <span className="text-sm font-medium">商品描述</span>
        <textarea className={`${input} min-h-32`} value={values.description} onChange={set("description")} maxLength={5000} />
      </label>

      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">
          商品圖片 *（{imageCount}/{MAX_IMAGES}，JPG / PNG / WebP，每張 8MB 內）
        </legend>
        <div className="grid grid-cols-4 gap-2">
          {kept.map((img) => (
            <div key={img.id} className="relative aspect-square overflow-hidden rounded bg-gray-100">
              {/* eslint-disable-next-line @next/next/no-img-element -- existing image */}
              <img src={img.url} alt="" className="h-full w-full object-cover" />
              <button
                type="button"
                onClick={() => setKept((k) => k.filter((x) => x.id !== img.id))}
                className="absolute right-1 top-1 rounded bg-black/60 px-1.5 text-xs text-white"
                aria-label="移除圖片"
              >
                ✕
              </button>
            </div>
          ))}
          {previews.map((url, i) => (
            <div key={url} className="relative aspect-square overflow-hidden rounded bg-gray-100">
              {/* eslint-disable-next-line @next/next/no-img-element -- local preview */}
              <img src={url} alt="" className="h-full w-full object-cover" />
              <button
                type="button"
                onClick={() => setFiles((f) => f.filter((_, j) => j !== i))}
                className="absolute right-1 top-1 rounded bg-black/60 px-1.5 text-xs text-white"
                aria-label="移除圖片"
              >
                ✕
              </button>
            </div>
          ))}
          {imageCount < MAX_IMAGES && (
            <label className="flex aspect-square cursor-pointer items-center justify-center rounded border-2 border-dashed text-2xl text-gray-400 hover:bg-gray-50">
              ＋
              <input
                type="file"
                accept={ACCEPT}
                multiple
                className="hidden"
                onChange={(e) => {
                  addFiles(e.target.files);
                  e.target.value = "";
                }}
              />
            </label>
          )}
        </div>
      </fieldset>

      <div className="grid grid-cols-3 gap-3">
        <label className="space-y-1">
          <span className="text-sm font-medium">起標價格</span>
          <input className={input} inputMode="numeric" value={values.start_price} onChange={set("start_price")} />
        </label>
        <label className="space-y-1">
          <span className="text-sm font-medium">最小加價</span>
          <input className={input} inputMode="numeric" value={values.min_increment} onChange={set("min_increment")} />
        </label>
        <label className="space-y-1">
          <span className="text-sm font-medium">最高加價</span>
          <input className={input} inputMode="numeric" value={values.max_increment} onChange={set("max_increment")} />
        </label>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-sm font-medium">開始時間（台北）</span>
          <input type="datetime-local" className={input} value={values.start_time} onChange={set("start_time")} />
        </label>
        <label className="space-y-1">
          <span className="text-sm font-medium">結束時間（台北）</span>
          <input type="datetime-local" className={input} value={values.end_time} onChange={set("end_time")} />
        </label>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-sm font-medium">寄送方式 *</span>
          <input className={input} value={values.shipping_method} onChange={set("shipping_method")} maxLength={100} />
        </label>
        <label className="space-y-1">
          <span className="text-sm font-medium">運費</span>
          <input className={input} inputMode="numeric" value={values.shipping_fee} onChange={set("shipping_fee")} />
        </label>
      </div>

      <label className="block space-y-1">
        <span className="text-sm font-medium">額外聯絡方式（僅賣家與得標者可見）</span>
        <textarea className={input} value={values.contact_info} onChange={set("contact_info")} maxLength={500} />
      </label>

      {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
      <button
        disabled={submitting}
        className="w-full rounded bg-emerald-600 px-4 py-2.5 font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
      >
        {submitting ? "儲存中…" : existing ? "儲存變更" : "發布"}
      </button>
    </form>
  );
}
