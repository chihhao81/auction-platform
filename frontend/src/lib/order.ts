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
