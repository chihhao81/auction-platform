"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { ApiError, api } from "@/lib/api";
import { submitPaymentForm } from "@/lib/order";
import { formatTaipei } from "@/lib/time";
import type { OrderDetail, PaymentCheckout } from "@/lib/types";

interface Props {
  order: OrderDetail;
  onChanged: () => void;
}

// What the viewer can do next. UX only: the backend checks role and status
// for every action.
export type OrderAction = "pay" | "shipping-info" | "wait-shipping-info" | "ship" | "receive" | "complete" | null;

export function nextAction(o: Pick<OrderDetail, "role" | "status" | "shipment">): OrderAction {
  const buyer = o.role === "buyer";
  switch (o.status) {
    case "AWAITING_PAYMENT":
      return buyer ? "pay" : null;
    case "AWAITING_SHIPMENT":
      if (buyer) return "shipping-info";
      return o.shipment?.recipient_address ? "ship" : "wait-shipping-info";
    case "SHIPPED":
      return buyer ? "receive" : null;
    case "RECEIVED":
      return buyer ? "complete" : null;
  }
  return null;
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3 rounded-xl bg-white p-4 shadow">
      <h2 className="font-bold">{title}</h2>
      {children}
    </section>
  );
}

const input = "w-full rounded border px-3 py-2";
const primary = "rounded bg-emerald-600 px-4 py-2 font-medium text-white hover:bg-emerald-700 disabled:opacity-50";

export function OrderActions({ order, onChanged }: Props) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const sh = order.shipment;
  const [addr, setAddr] = useState({
    recipient_name: sh?.recipient_name ?? "",
    recipient_phone: sh?.recipient_phone ?? "",
    recipient_address: sh?.recipient_address ?? "",
  });
  const [ship, setShip] = useState({ carrier: "", tracking_number: "" });

  async function run(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
      onChanged();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "操作失敗，請稍後再試");
    } finally {
      setBusy(false);
    }
  }

  async function pay() {
    setBusy(true);
    setError(null);
    try {
      const co = await api.post<PaymentCheckout>(`/api/orders/${order.id}/payments`);
      submitPaymentForm(co.action, co.fields); // navigates away to ECPay
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "無法建立付款");
      setBusy(false);
    }
  }

  const action = nextAction(order);
  const err = error && <p role="alert" className="text-sm text-red-600">{error}</p>;

  const shipmentInfo = sh && (sh.carrier || sh.recipient_address || sh.purged) && (
    <Panel title="寄送資訊">
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        {sh.purged ? (
          <>
            <dt className="text-gray-500">收件資訊</dt>
            <dd>已依隱私政策刪除</dd>
          </>
        ) : (
          sh.recipient_address && (
            <>
              <dt className="text-gray-500">收件人</dt>
              <dd>{sh.recipient_name}</dd>
              <dt className="text-gray-500">電話</dt>
              <dd>{sh.recipient_phone}</dd>
              <dt className="text-gray-500">地址 / 門市</dt>
              <dd className="whitespace-pre-wrap break-words">{sh.recipient_address}</dd>
            </>
          )
        )}
        {sh.carrier && (
          <>
            <dt className="text-gray-500">物流</dt>
            <dd>{sh.carrier}</dd>
            <dt className="text-gray-500">單號</dt>
            <dd className="break-all">{sh.tracking_number}</dd>
            {sh.shipped_at && (
              <>
                <dt className="text-gray-500">出貨時間</dt>
                <dd>{formatTaipei(sh.shipped_at)}</dd>
              </>
            )}
          </>
        )}
      </dl>
    </Panel>
  );

  let panel: ReactNode = null;
  switch (action) {
    case "pay":
      panel = (
        <Panel title="付款">
          <p className="text-sm text-gray-600">將前往綠界付款頁面。付款結果以綠界通知後端為準，完成後此頁會自動更新。</p>
          <button onClick={pay} disabled={busy} className={primary}>
            {busy ? "前往付款中…" : "前往付款"}
          </button>
          {err}
        </Panel>
      );
      break;
    case "shipping-info":
      panel = (
        <Panel title={sh?.recipient_address ? "收件資訊（出貨前可修改）" : "請填寫收件資訊"}>
          <form
            className="space-y-2"
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              void run(() => api.put(`/api/orders/${order.id}/shipping-info`, addr));
            }}
          >
            <input className={input} placeholder="收件人姓名" maxLength={50} value={addr.recipient_name}
              onChange={(e) => setAddr({ ...addr, recipient_name: e.target.value })} aria-label="收件人姓名" />
            <input className={input} placeholder="電話" inputMode="tel" maxLength={20} value={addr.recipient_phone}
              onChange={(e) => setAddr({ ...addr, recipient_phone: e.target.value })} aria-label="電話" />
            <textarea className={input} placeholder="收件地址或超商門市（名稱與店號）" maxLength={300} value={addr.recipient_address}
              onChange={(e) => setAddr({ ...addr, recipient_address: e.target.value })} aria-label="收件地址" />
            <p className="text-xs text-gray-500">僅此訂單的賣家看得到，訂單完成 30 天後自動刪除。</p>
            <button disabled={busy} className={primary}>儲存收件資訊</button>
            {err}
          </form>
        </Panel>
      );
      break;
    case "wait-shipping-info":
      panel = (
        <Panel title="等待買家填寫收件資訊">
          <p className="text-sm text-gray-600">買家已付款，填寫收件資訊後即可出貨。</p>
        </Panel>
      );
      break;
    case "ship":
      panel = (
        <Panel title="出貨">
          <form
            className="space-y-2"
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              void run(() => api.post(`/api/orders/${order.id}/ship`, ship));
            }}
          >
            <input className={input} placeholder="物流方式（例：黑貓宅急便）" maxLength={50} value={ship.carrier}
              onChange={(e) => setShip({ ...ship, carrier: e.target.value })} aria-label="物流方式" />
            <input className={input} placeholder="物流單號" maxLength={100} value={ship.tracking_number}
              onChange={(e) => setShip({ ...ship, tracking_number: e.target.value })} aria-label="物流單號" />
            <button disabled={busy} className={primary}>確認出貨</button>
            {err}
          </form>
        </Panel>
      );
      break;
    case "receive":
    case "complete":
      panel = (
        <Panel title={action === "receive" ? "確認收貨" : "完成訂單"}>
          {order.auto_advance_at && (
            <p className="text-sm text-gray-600">
              若未操作，系統將於 {formatTaipei(order.auto_advance_at)} 自動{action === "receive" ? "確認收貨" : "完成訂單"}。
            </p>
          )}
          <button
            disabled={busy}
            className={primary}
            onClick={() => void run(() => api.post(`/api/orders/${order.id}/${action}`))}
          >
            {action === "receive" ? "我已收到商品" : "完成訂單"}
          </button>
          {err}
        </Panel>
      );
      break;
  }

  return (
    <>
      {panel}
      {shipmentInfo}
    </>
  );
}
