"use client";

import { useState, type FormEvent } from "react";
import { ApiError } from "@/lib/api";

interface Props {
  label: string;
  danger?: boolean;
  placeholder?: string;
  onConfirm: (reason: string) => Promise<unknown>;
  onDone: () => void;
}

// A button that expands into a "reason" input before confirming. Every admin
// action that changes data requires a reason, which goes to the audit log.
export function ReasonAction({ label, danger, placeholder = "原因（必填）", onConfirm, onDone }: Props) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!reason.trim()) {
      setError("請填寫原因");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await onConfirm(reason.trim());
      setOpen(false);
      setReason("");
      onDone();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "操作失敗");
    } finally {
      setBusy(false);
    }
  }

  const color = danger ? "border-red-600 text-red-700" : "border-gray-400 text-gray-700";
  if (!open) {
    return (
      <button onClick={() => setOpen(true)} className={`rounded border px-2 py-0.5 text-xs ${color}`}>
        {label}
      </button>
    );
  }
  return (
    <form onSubmit={submit} className="flex flex-wrap items-center gap-1">
      <input
        autoFocus
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        maxLength={500}
        placeholder={placeholder}
        aria-label={`${label}原因`}
        className="min-w-0 flex-1 rounded border px-2 py-0.5 text-xs"
      />
      <button disabled={busy} className={`rounded border px-2 py-0.5 text-xs ${color}`}>
        確認{label}
      </button>
      <button type="button" onClick={() => setOpen(false)} className="px-1 text-xs text-gray-500">
        取消
      </button>
      {error && <span role="alert" className="w-full text-xs text-red-600">{error}</span>}
    </form>
  );
}
