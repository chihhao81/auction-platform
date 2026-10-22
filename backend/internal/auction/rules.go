// Package auction implements auctions and bidding.
//
// rules.go holds the pure business rules (no I/O) so they can be unit tested
// exhaustively; service.go applies them inside database transactions.
package auction

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/textutil"
)

type Status string

const (
	StatusUpcoming Status = "UPCOMING"
	StatusActive   Status = "ACTIVE"
	StatusEnded    Status = "ENDED"
	StatusClosed   Status = "CLOSED"
)

const (
	// ClosedAfter is how long an ended auction stays visible as ENDED.
	ClosedAfter = 24 * time.Hour
	// ExtensionWindow / ExtensionAmount implement anti-sniping: a valid bid
	// received with <= 1 minute left pushes end_time back by 2 minutes.
	ExtensionWindow = time.Minute
	ExtensionAmount = 2 * time.Minute

	MaxOpenAuctionsPerSeller = 5
	MaxImages                = 4
	MinImages                = 1

	DefaultStartPrice   int64 = 0
	DefaultMinIncrement int64 = 50
	DefaultMaxIncrement int64 = 50
	DefaultDuration           = 48 * time.Hour

	maxPrice       int64 = 10_000_000
	maxIncrement   int64 = 1_000_000
	maxShippingFee int64 = 100_000
	minDuration          = 10 * time.Minute
	maxDuration          = 14 * 24 * time.Hour
	maxStartAhead        = 30 * 24 * time.Hour
	startTolerance       = time.Minute
)

// StatusAt derives the auction status at instant now.
func StatusAt(start, end, now time.Time) Status {
	switch {
	case now.Before(start):
		return StatusUpcoming
	case now.Before(end):
		return StatusActive
	case now.Before(end.Add(ClosedAfter)):
		return StatusEnded
	default:
		return StatusClosed
	}
}

// StatusOf is the auction's status at now, including an admin close, which
// makes an auction CLOSED immediately regardless of its times.
func StatusOf(a db.Auction, now time.Time) Status {
	if a.ClosedAt != nil {
		return StatusClosed
	}
	return StatusAt(a.StartTime, a.EndTime, now)
}

// BidRange returns the inclusive range of valid next bids. Before the first
// bid current_price equals start_price, so the same formula applies.
func BidRange(a db.Auction) (min, max int64) {
	return a.CurrentPrice + a.MinIncrement, a.CurrentPrice + a.MaxIncrement
}

var (
	ErrNotStarted     = apperr.Conflict("AUCTION_NOT_STARTED", "競標尚未開始")
	ErrAuctionEnded   = apperr.Conflict("AUCTION_ENDED", "競標已結束")
	ErrSellerBid      = apperr.Forbidden("SELLER_CANNOT_BID", "賣家不可對自己的商品出價")
	ErrAlreadyHighest = apperr.Conflict("ALREADY_HIGHEST_BIDDER", "您目前已是最高出價者")
)

// ValidateBid checks a bid against the locked auction row at instant now.
func ValidateBid(a db.Auction, bidder uuid.UUID, amount int64, now time.Time) error {
	switch StatusOf(a, now) {
	case StatusUpcoming:
		return ErrNotStarted
	case StatusEnded, StatusClosed:
		return ErrAuctionEnded
	}
	if bidder == a.SellerID {
		return ErrSellerBid
	}
	if a.HighestBidderID.Valid && a.HighestBidderID.UUID == bidder {
		return ErrAlreadyHighest
	}
	lo, hi := BidRange(a)
	if amount < lo || amount > hi {
		return apperr.Invalid("BID_OUT_OF_RANGE", fmt.Sprintf("出價需介於 %d ~ %d", lo, hi))
	}
	return nil
}

// ExtendedEnd returns the end time after a valid bid at instant now.
func ExtendedEnd(end, now time.Time) (time.Time, bool) {
	if end.Sub(now) <= ExtensionWindow {
		return end.Add(ExtensionAmount), true
	}
	return end, false
}

// DefaultStart is the next full hour after now.
func DefaultStart(now time.Time) time.Time {
	return now.UTC().Truncate(time.Hour).Add(time.Hour)
}

// Input is the editable auction content as submitted by a seller. Pointer
// fields are optional and fall back to the documented defaults.
type Input struct {
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	StartPrice     *int64     `json:"start_price"`
	MinIncrement   *int64     `json:"min_increment"`
	MaxIncrement   *int64     `json:"max_increment"`
	StartTime      *time.Time `json:"start_time"`
	EndTime        *time.Time `json:"end_time"`
	ContactInfo    string     `json:"contact_info"`
	ShippingMethod string     `json:"shipping_method"`
	ShippingFee    int64      `json:"shipping_fee"`
}

// Fields is validated, defaulted auction content.
type Fields struct {
	Title          string
	Description    string
	StartPrice     int64
	MinIncrement   int64
	MaxIncrement   int64
	StartTime      time.Time
	EndTime        time.Time
	ContactInfo    string
	ShippingMethod string
	ShippingFee    int64
}

func invalid(code, msg string) error { return apperr.Invalid(code, msg) }

// Normalize applies defaults and validates in at instant now.
func (in Input) Normalize(now time.Time) (Fields, error) {
	f := Fields{
		StartPrice: DefaultStartPrice, MinIncrement: DefaultMinIncrement, MaxIncrement: DefaultMaxIncrement,
		ShippingFee: in.ShippingFee,
	}
	var ok bool
	if f.Title, ok = textutil.Clean(in.Title, 100, false); !ok || f.Title == "" {
		return Fields{}, invalid("INVALID_TITLE", "商品名稱必填，且不可超過 100 字")
	}
	if f.Description, ok = textutil.Clean(in.Description, 5000, true); !ok {
		return Fields{}, invalid("INVALID_DESCRIPTION", "商品描述不可超過 5000 字")
	}
	if f.ContactInfo, ok = textutil.Clean(in.ContactInfo, 500, true); !ok {
		return Fields{}, invalid("INVALID_CONTACT_INFO", "額外聯絡方式不可超過 500 字")
	}
	if f.ShippingMethod, ok = textutil.Clean(in.ShippingMethod, 100, false); !ok || f.ShippingMethod == "" {
		return Fields{}, invalid("INVALID_SHIPPING_METHOD", "寄送方式必填，且不可超過 100 字")
	}
	if in.StartPrice != nil {
		f.StartPrice = *in.StartPrice
	}
	if in.MinIncrement != nil {
		f.MinIncrement = *in.MinIncrement
	}
	if in.MaxIncrement != nil {
		f.MaxIncrement = *in.MaxIncrement
	}
	if f.StartPrice < 0 || f.StartPrice > maxPrice {
		return Fields{}, invalid("INVALID_START_PRICE", fmt.Sprintf("起標價格需介於 0 ~ %d", maxPrice))
	}
	if f.MinIncrement < 1 || f.MinIncrement > maxIncrement {
		return Fields{}, invalid("INVALID_MIN_INCREMENT", fmt.Sprintf("最小加價需介於 1 ~ %d", maxIncrement))
	}
	if f.MaxIncrement < f.MinIncrement || f.MaxIncrement > maxIncrement {
		return Fields{}, invalid("INVALID_MAX_INCREMENT", "最高加價不可小於最小加價")
	}
	if f.ShippingFee < 0 || f.ShippingFee > maxShippingFee {
		return Fields{}, invalid("INVALID_SHIPPING_FEE", fmt.Sprintf("運費需介於 0 ~ %d", maxShippingFee))
	}

	f.StartTime = DefaultStart(now)
	if in.StartTime != nil {
		f.StartTime = in.StartTime.UTC().Truncate(time.Second)
	}
	f.EndTime = f.StartTime.Add(DefaultDuration)
	if in.EndTime != nil {
		f.EndTime = in.EndTime.UTC().Truncate(time.Second)
	}
	if f.StartTime.Before(now.Add(-startTolerance)) {
		return Fields{}, invalid("INVALID_START_TIME", "開始時間不可早於現在")
	}
	if f.StartTime.After(now.Add(maxStartAhead)) {
		return Fields{}, invalid("INVALID_START_TIME", "開始時間不可超過 30 天後")
	}
	d := f.EndTime.Sub(f.StartTime)
	if d < minDuration || d > maxDuration {
		return Fields{}, invalid("INVALID_END_TIME", "競標時間長度需介於 10 分鐘 ~ 14 天")
	}
	return f, nil
}
