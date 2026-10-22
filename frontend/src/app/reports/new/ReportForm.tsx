"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ApiError, api } from "@/lib/api";
import type { ReportReasons } from "@/lib/types";

export function ReportForm() {
  const params = useSearchParams();
  const router = useRouter();
  const type = params.get("type") === "AUCTION" ? "AUCTION" : "USER";
  const targetId = params.get("id") ?? "";
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const [reason, setReason] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.get<ReportReasons>("/api/reports/reasons").then((r) => setReasons(r[type]), () => {});
  }, [type]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!reason) {
      setError("請選擇檢舉原因");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.post("/api/reports", { target_type: type, target_id: targetId, reason, description });
      router.replace("/me#my-reports");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "送出失敗");
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="mx-auto max-w-lg space-y-3 rounded-xl bg-white p-5 shadow">
      <h1 className="text-xl font-bold">檢舉{type === "AUCTION" ? "競標" : "使用者"}</h1>
      <fieldset className="space-y-1">
        <legend className="text-sm font-medium">原因</legend>
        {Object.entries(reasons).map(([key, label]) => (
          <label key={key} className="flex items-center gap-2 text-sm">
            <input type="radio" name="reason" value={key} checked={reason === key} onChange={() => setReason(key)} />
            {label}
          </label>
        ))}
      </fieldset>
      <label className="block space-y-1">
        <span className="text-sm font-medium">說明（選填）</span>
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          maxLength={2000}
          className="min-h-28 w-full rounded border px-3 py-2"
        />
      </label>
      {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
      <button disabled={busy} className="w-full rounded bg-red-600 px-4 py-2 font-medium text-white disabled:opacity-50">
        送出檢舉
      </button>
    </form>
  );
}
