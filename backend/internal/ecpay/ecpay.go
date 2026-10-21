// Package ecpay implements the parts of the ECPay (綠界) AIO payment API the
// platform uses: building a signed checkout form, verifying signed results
// (server callback and trade query) and querying a trade's status.
//
// Nothing a browser sends is trusted: a payment only counts when its result
// carries a valid CheckMacValue computed with the backend-only HashKey/HashIV.
package ecpay

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	StageBaseURL = "https://payment-stage.ecpay.com.tw"
	ProdBaseURL  = "https://payment.ecpay.com.tw"
)

// ECPay interprets MerchantTradeDate in Taiwan time.
var taipei = time.FixedZone("Asia/Taipei", 8*3600)

type Client struct {
	MerchantID string
	HashKey    string
	HashIV     string
	BaseURL    string
	HTTP       *http.Client
}

func New(merchantID, hashKey, hashIV, baseURL string) *Client {
	return &Client{
		MerchantID: merchantID, HashKey: hashKey, HashIV: hashIV, BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{Timeout: 15 * time.Second},
	}
}

// dotnetEscape reproduces the .NET HttpUtility.UrlEncode output ECPay signs
// against: like form encoding (space -> "+"), but "-_.!*()" stay literal and
// "~" is escaped.
func dotnetEscape(s string) string {
	e := url.QueryEscape(s)
	return strings.NewReplacer("%21", "!", "%2A", "*", "%28", "(", "%29", ")", "~", "%7E").Replace(e)
}

// CheckMac computes the SHA256 CheckMacValue of params (any existing
// CheckMacValue entry is ignored).
func (c *Client) CheckMac(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "CheckMacValue" {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
	var b strings.Builder
	b.WriteString("HashKey=" + c.HashKey)
	for _, k := range keys {
		b.WriteString("&" + k + "=" + params[k])
	}
	b.WriteString("&HashIV=" + c.HashIV)
	sum := sha256.Sum256([]byte(strings.ToLower(dotnetEscape(b.String()))))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// Verify reports whether params carry a valid CheckMacValue and belong to
// this merchant.
func (c *Client) Verify(params map[string]string) bool {
	got := params["CheckMacValue"]
	if got == "" || params["MerchantID"] != c.MerchantID {
		return false
	}
	want := c.CheckMac(params)
	return subtle.ConstantTimeCompare([]byte(strings.ToUpper(got)), []byte(want)) == 1
}

// FormToMap flattens form values (ECPay never repeats keys).
func FormToMap(v url.Values) map[string]string {
	m := make(map[string]string, len(v))
	for k := range v {
		m[k] = v.Get(k)
	}
	return m
}

// Checkout describes one payment attempt.
type Checkout struct {
	MerchantTradeNo string // unique, alphanumeric, <= 20 chars
	Amount          int64
	ItemName        string
	TradeDesc       string
	Time            time.Time
	ReturnURL       string // server-to-server result notification
	ClientBackURL   string // where the buyer's browser returns
}

// CheckoutForm returns the action URL and signed fields of an auto-submitted
// POST form that sends the buyer to ECPay's payment page.
func (c *Client) CheckoutForm(co Checkout) (string, map[string]string) {
	f := map[string]string{
		"MerchantID":        c.MerchantID,
		"MerchantTradeNo":   co.MerchantTradeNo,
		"MerchantTradeDate": co.Time.In(taipei).Format("2006/01/02 15:04:05"),
		"PaymentType":       "aio",
		"TotalAmount":       strconv.FormatInt(co.Amount, 10),
		"TradeDesc":         co.TradeDesc,
		"ItemName":          co.ItemName,
		"ReturnURL":         co.ReturnURL,
		"ClientBackURL":     co.ClientBackURL,
		"ChoosePayment":     "ALL",
		"EncryptType":       "1",
	}
	f["CheckMacValue"] = c.CheckMac(f)
	return c.BaseURL + "/Cashier/AioCheckOut/V5", f
}

// Result is a verified payment result, from either the server callback or
// a trade query.
type Result struct {
	MerchantTradeNo string
	TradeNo         string
	Amount          int64
	Paid            bool
	Simulated       bool // ECPay "simulate paid" test notification
	PaymentType     string
	PaymentDate     string
	RtnMsg          string
}

var ErrBadSignature = errors.New("ecpay: invalid CheckMacValue")

// ParseCallback verifies and parses the ReturnURL notification.
func (c *Client) ParseCallback(params map[string]string) (Result, error) {
	if !c.Verify(params) {
		return Result{}, ErrBadSignature
	}
	amt, err := strconv.ParseInt(params["TradeAmt"], 10, 64)
	if err != nil {
		return Result{}, fmt.Errorf("ecpay: bad TradeAmt %q", params["TradeAmt"])
	}
	return Result{
		MerchantTradeNo: params["MerchantTradeNo"], TradeNo: params["TradeNo"], Amount: amt,
		Paid: params["RtnCode"] == "1", Simulated: params["SimulatePaid"] == "1",
		PaymentType: params["PaymentType"], PaymentDate: params["PaymentDate"], RtnMsg: params["RtnMsg"],
	}, nil
}

// QueryTrade asks ECPay for the current status of a trade. It is used when
// the callback is delayed or lost (e.g. the buyer returns to the site before
// ECPay notifies the backend).
func (c *Client) QueryTrade(ctx context.Context, merchantTradeNo string, now time.Time) (Result, error) {
	p := map[string]string{
		"MerchantID":      c.MerchantID,
		"MerchantTradeNo": merchantTradeNo,
		"TimeStamp":       strconv.FormatInt(now.Unix(), 10),
	}
	p["CheckMacValue"] = c.CheckMac(p)
	form := url.Values{}
	for k, v := range p {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/Cashier/QueryTradeInfo/V5", strings.NewReader(form.Encode()))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ecpay query: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Result{}, fmt.Errorf("ecpay query: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("ecpay query: status %d", resp.StatusCode)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return Result{}, fmt.Errorf("ecpay query: parse response: %w", err)
	}
	m := FormToMap(values)
	if !c.Verify(m) {
		return Result{}, ErrBadSignature
	}
	if m["MerchantTradeNo"] != merchantTradeNo {
		return Result{}, fmt.Errorf("ecpay query: trade number mismatch")
	}
	amt, _ := strconv.ParseInt(m["TradeAmt"], 10, 64)
	return Result{
		MerchantTradeNo: m["MerchantTradeNo"], TradeNo: m["TradeNo"], Amount: amt,
		Paid: m["TradeStatus"] == "1", PaymentType: m["PaymentType"], PaymentDate: m["PaymentDate"],
	}, nil
}
