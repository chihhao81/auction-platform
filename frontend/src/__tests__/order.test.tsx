import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { nextAction } from "@/app/orders/[id]/OrderActions";
import { OrderMessages } from "@/app/orders/[id]/OrderMessages";
import { submitPaymentForm } from "@/lib/order";
import type { Shipment } from "@/lib/types";

const shipment = (addr: string): Shipment => ({
  recipient_name: "a",
  recipient_phone: "1",
  recipient_address: addr,
  carrier: "",
  tracking_number: "",
  shipped_at: null,
  purged: false,
});

describe("nextAction", () => {
  it("follows the order state machine per role", () => {
    expect(nextAction({ role: "buyer", status: "AWAITING_CONTACT", shipment: null })).toBeNull();
    expect(nextAction({ role: "buyer", status: "AWAITING_PAYMENT", shipment: null })).toBe("pay");
    expect(nextAction({ role: "seller", status: "AWAITING_PAYMENT", shipment: null })).toBeNull();
    expect(nextAction({ role: "buyer", status: "AWAITING_SHIPMENT", shipment: null })).toBe("shipping-info");
    expect(nextAction({ role: "seller", status: "AWAITING_SHIPMENT", shipment: null })).toBe("wait-shipping-info");
    expect(nextAction({ role: "seller", status: "AWAITING_SHIPMENT", shipment: shipment("台北") })).toBe("ship");
    expect(nextAction({ role: "buyer", status: "SHIPPED", shipment: null })).toBe("receive");
    expect(nextAction({ role: "seller", status: "SHIPPED", shipment: null })).toBeNull();
    expect(nextAction({ role: "buyer", status: "RECEIVED", shipment: null })).toBe("complete");
    expect(nextAction({ role: "buyer", status: "COMPLETED", shipment: null })).toBeNull();
  });
});

describe("submitPaymentForm", () => {
  it("posts the signed fields to ECPay", () => {
    const submit = vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(() => {});
    const form = submitPaymentForm("https://payment-stage.ecpay.com.tw/Cashier/AioCheckOut/V5", {
      MerchantTradeNo: "CB1",
      CheckMacValue: "ABC",
    });
    expect(form.method).toBe("post");
    expect(form.action).toBe("https://payment-stage.ecpay.com.tw/Cashier/AioCheckOut/V5");
    expect((form.elements.namedItem("CheckMacValue") as HTMLInputElement).value).toBe("ABC");
    expect(submit).toHaveBeenCalled();
    submit.mockRestore();
  });

  it("refuses non-ECPay targets", () => {
    expect(() => submitPaymentForm("https://evil.example/pay", {})).toThrow();
    expect(() => submitPaymentForm("http://payment.ecpay.com.tw/x", {})).toThrow();
    expect(() => submitPaymentForm("https://ecpay.com.tw.evil.example/x", {})).toThrow();
  });
});

describe("OrderMessages", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("renders messages as text and sends new ones", async () => {
    document.cookie = "csrf_token=t";
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return new Response(JSON.stringify({ message: { id: "m2", mine: true, content: "我要付款", created_at: "2026-10-22T00:00:00Z", read_at: null } }), { status: 201 });
      }
      return new Response(JSON.stringify({ items: [{ id: "m1", mine: false, content: "<b>hi</b>", created_at: "2026-10-22T00:00:00Z", read_at: null }] }));
    });
    vi.stubGlobal("fetch", fetchMock);
    const onSent = vi.fn();
    const { container } = render(<OrderMessages orderId="o1" disabled={false} onSent={onSent} />);

    expect(await screen.findByText("<b>hi</b>")).toBeInTheDocument();
    expect(container.querySelector("b")).toBeNull();

    await userEvent.type(screen.getByLabelText("訊息內容"), "我要付款");
    await userEvent.click(screen.getByRole("button", { name: "傳送" }));
    expect(await screen.findByText("我要付款", { selector: "p" })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/api/orders/o1/messages", expect.objectContaining({ method: "POST", body: '{"content":"我要付款"}' }));
    expect(onSent).toHaveBeenCalled();
  });

  it("hides the input when messaging is disabled", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ items: [] }))));
    render(<OrderMessages orderId="o1" disabled onSent={() => {}} />);
    expect(await screen.findByText(/尚無訊息/)).toBeInTheDocument();
    expect(screen.queryByLabelText("訊息內容")).toBeNull();
  });
});
