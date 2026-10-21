import type { OrderStatus } from "./types";

export const ORDER_STATUS_LABELS: Record<OrderStatus, string> = {
  AWAITING_CONTACT: "等待聯絡",
  AWAITING_PAYMENT: "等待付款",
  PAID: "已付款",
  AWAITING_SHIPMENT: "等待出貨",
  SHIPPED: "已出貨",
  RECEIVED: "已收貨",
  COMPLETED: "已完成",
  CANCELLED: "已取消",
  DISPUTED: "爭議處理中",
};

// Builds and submits the POST form that hands the buyer over to ECPay. The
// fields are produced and signed by the backend; nothing here affects the
// amount. Only ECPay's https origin is accepted as the form target.
export function submitPaymentForm(action: string, fields: Record<string, string>, doc: Document = document): HTMLFormElement {
  const url = new URL(action);
  if (url.protocol !== "https:" || !/(^|\.)ecpay\.com\.tw$/.test(url.hostname)) {
    throw new Error("unexpected payment endpoint");
  }
  const form = doc.createElement("form");
  form.method = "POST";
  form.action = url.href;
  for (const [name, value] of Object.entries(fields)) {
    const input = doc.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.appendChild(input);
  }
  doc.body.appendChild(form);
  form.submit();
  return form;
}
