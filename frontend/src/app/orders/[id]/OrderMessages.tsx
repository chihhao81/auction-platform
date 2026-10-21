"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { ApiError, api } from "@/lib/api";
import { usePolling } from "@/lib/hooks";
import { formatTaipei } from "@/lib/time";
import type { OrderMessage } from "@/lib/types";

const MAX_LENGTH = 1000;
const POLL_MS = 10_000;

interface Props {
  orderId: string;
  disabled: boolean;
  // Called after sending, since the buyer's first message changes the order status.
  onSent: () => void;
}

export function OrderMessages({ orderId, disabled, onSent }: Props) {
  const [messages, setMessages] = useState<OrderMessage[]>([]);
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const bottom = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    try {
      const res = await api.get<{ items: OrderMessage[] }>(`/api/orders/${orderId}/messages`);
      setMessages(res.items);
    } catch {
      // next poll retries
    }
  }, [orderId]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- initial fetch
    void load();
  }, [load]);
  usePolling(load, POLL_MS);

  useEffect(() => {
    bottom.current?.scrollIntoView?.({ block: "nearest" });
  }, [messages.length]);

  async function send(e: FormEvent) {
    e.preventDefault();
    const content = text.trim();
    if (!content) return;
    setSending(true);
    setError(null);
    try {
      const res = await api.post<{ message: OrderMessage }>(`/api/orders/${orderId}/messages`, { content });
      setMessages((m) => [...m, res.message]);
      setText("");
      onSent();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "傳送失敗");
    } finally {
      setSending(false);
    }
  }

  return (
    <section className="space-y-3 rounded-xl bg-white p-4 shadow">
      <h2 className="font-bold">訂單訊息</h2>
      <div className="max-h-96 space-y-2 overflow-y-auto">
        {messages.length === 0 && <p className="text-sm text-gray-500">尚無訊息。買家請透過此處聯絡賣家。</p>}
        {messages.map((m) => (
          <div key={m.id} className={`flex ${m.mine ? "justify-end" : "justify-start"}`}>
            <div className={`max-w-[80%] rounded-lg px-3 py-2 text-sm ${m.mine ? "bg-emerald-600 text-white" : "bg-gray-100"}`}>
              {/* Plain text: React escapes it; HTML is never interpreted. */}
              <p className="whitespace-pre-wrap break-words">{m.content}</p>
              <p className={`mt-1 text-[10px] ${m.mine ? "text-emerald-100" : "text-gray-400"}`}>
                {formatTaipei(m.created_at)}
                {m.mine && m.read_at && " · 已讀"}
              </p>
            </div>
          </div>
        ))}
        <div ref={bottom} />
      </div>
      {!disabled && (
        <form onSubmit={send} className="space-y-1">
          <div className="flex gap-2">
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              maxLength={MAX_LENGTH}
              rows={2}
              className="min-w-0 flex-1 rounded border px-3 py-2 text-sm"
              placeholder="輸入訊息（純文字）"
              aria-label="訊息內容"
            />
            <button disabled={sending || !text.trim()} className="rounded bg-emerald-600 px-4 text-white disabled:opacity-50">
              傳送
            </button>
          </div>
          <p className="text-right text-xs text-gray-400">
            {text.length}/{MAX_LENGTH}
          </p>
          {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
        </form>
      )}
    </section>
  );
}
