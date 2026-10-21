package ecpay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// ECPay's public test merchant (published in their developer docs).
func testClient() *Client {
	return New("3002607", "pwFHCqoQZGmho4w6", "EkRm7iFT261dpevs", StageBaseURL)
}

// The example from ECPay's CheckMacValue documentation (developers.ecpay.com.tw/?p=2902).
func TestCheckMacMatchesOfficialExample(t *testing.T) {
	params := map[string]string{
		"ChoosePayment":     "ALL",
		"EncryptType":       "1",
		"ItemName":          "Apple iphone 15",
		"MerchantID":        "3002607",
		"MerchantTradeDate": "2023/03/12 15:30:23",
		"MerchantTradeNo":   "ecpay20230312153023",
		"PaymentType":       "aio",
		"ReturnURL":         "https://www.ecpay.com.tw/receive.php",
		"TotalAmount":       "30000",
		"TradeDesc":         "促銷方案",
	}
	const want = "6C51C9E6888DE861FD62FB1DD17029FC742634498FD813DC43D4243B5685B840"
	if got := testClient().CheckMac(params); got != want {
		t.Fatalf("CheckMac = %s, want %s", got, want)
	}
}

func TestDotnetEscape(t *testing.T) {
	cases := map[string]string{
		"a b":  "a+b",
		"-_.":  "-_.",
		"!*()": "!*()",
		"~":    "%7E",
		"/:&=": "%2F%3A%26%3D",
		"守":    "%E5%AE%88",
	}
	for in, want := range cases {
		if got := dotnetEscape(in); got != want {
			t.Errorf("dotnetEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func signed(c *Client, p map[string]string) map[string]string {
	p["CheckMacValue"] = c.CheckMac(p)
	return p
}

func TestParseCallback(t *testing.T) {
	c := testClient()
	ok := signed(c, map[string]string{
		"MerchantID": "3002607", "MerchantTradeNo": "CB123", "RtnCode": "1", "RtnMsg": "交易成功",
		"TradeNo": "2410210000001", "TradeAmt": "1675", "PaymentType": "Credit_CreditCard",
		"PaymentDate": "2026/10/21 23:40:00", "SimulatePaid": "0",
	})
	r, err := c.ParseCallback(ok)
	if err != nil || !r.Paid || r.Amount != 1675 || r.MerchantTradeNo != "CB123" || r.Simulated {
		t.Fatalf("result %+v err %v", r, err)
	}

	tampered := map[string]string{}
	for k, v := range ok {
		tampered[k] = v
	}
	tampered["TradeAmt"] = "1"
	if _, err := c.ParseCallback(tampered); err != ErrBadSignature {
		t.Fatalf("tampered amount must fail signature, got %v", err)
	}

	other := signed(New("9999999", c.HashKey, c.HashIV, ""), map[string]string{"MerchantID": "9999999", "TradeAmt": "1"})
	if _, err := c.ParseCallback(other); err != ErrBadSignature {
		t.Fatal("other merchant must be rejected")
	}
	if _, err := c.ParseCallback(map[string]string{"MerchantID": "3002607", "TradeAmt": "1"}); err != ErrBadSignature {
		t.Fatal("missing signature must be rejected")
	}
}

func TestCheckoutFormIsSignedInTaipeiTime(t *testing.T) {
	c := testClient()
	action, f := c.CheckoutForm(Checkout{
		MerchantTradeNo: "CB1", Amount: 500, ItemName: "守宮", TradeDesc: "Critter Bid",
		Time: time.Date(2026, 10, 21, 16, 30, 0, 0, time.UTC), ReturnURL: "https://x/notify", ClientBackURL: "https://x/back",
	})
	if action != StageBaseURL+"/Cashier/AioCheckOut/V5" {
		t.Errorf("action %s", action)
	}
	if f["MerchantTradeDate"] != "2026/10/22 00:30:00" {
		t.Errorf("trade date %s", f["MerchantTradeDate"])
	}
	if !c.Verify(f) {
		t.Error("form signature does not verify")
	}
}

func TestQueryTrade(t *testing.T) {
	c := testClient()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if !c.Verify(FormToMap(r.PostForm)) || r.URL.Path != "/Cashier/QueryTradeInfo/V5" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp := signed(c, map[string]string{
			"MerchantID": "3002607", "MerchantTradeNo": r.PostForm.Get("MerchantTradeNo"),
			"TradeNo": "T1", "TradeAmt": "800", "TradeStatus": "1", "PaymentType": "Credit_CreditCard",
		})
		v := url.Values{}
		for k, val := range resp {
			v.Set(k, val)
		}
		_, _ = w.Write([]byte(v.Encode()))
	}))
	defer srv.Close()
	c.BaseURL = srv.URL
	r, err := c.QueryTrade(context.Background(), "CB9", time.Now())
	if err != nil || !r.Paid || r.Amount != 800 || r.TradeNo != "T1" {
		t.Fatalf("result %+v err %v", r, err)
	}
}

func TestQueryTradeRejectsUnsignedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("MerchantID=3002607&MerchantTradeNo=CB9&TradeStatus=1&TradeAmt=800&CheckMacValue=FAKE"))
	}))
	defer srv.Close()
	c := testClient()
	c.BaseURL = srv.URL
	if _, err := c.QueryTrade(context.Background(), "CB9", time.Now()); err != ErrBadSignature {
		t.Fatalf("got %v", err)
	}
}
