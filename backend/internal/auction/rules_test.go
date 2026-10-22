package auction

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/database/db"
)

var (
	t0     = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	seller = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	alice  = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	bob    = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
)

func activeAuction() db.Auction {
	return db.Auction{
		ID: uuid.New(), SellerID: seller,
		StartPrice: 1000, CurrentPrice: 1000, MinIncrement: 50, MaxIncrement: 500,
		StartTime: t0.Add(-time.Hour), EndTime: t0.Add(time.Hour), OriginalEndTime: t0.Add(time.Hour),
	}
}

func TestStatusAt(t *testing.T) {
	start, end := t0, t0.Add(time.Hour)
	cases := []struct {
		now  time.Time
		want Status
	}{
		{start.Add(-time.Second), StatusUpcoming},
		{start, StatusActive},
		{end.Add(-time.Nanosecond), StatusActive},
		{end, StatusEnded},
		{end.Add(24*time.Hour - time.Second), StatusEnded},
		{end.Add(24 * time.Hour), StatusClosed},
	}
	for _, tc := range cases {
		if got := StatusAt(start, end, tc.now); got != tc.want {
			t.Errorf("StatusAt(now=%v) = %s, want %s", tc.now.Sub(start), got, tc.want)
		}
	}
}

func TestValidateBid(t *testing.T) {
	withHighest := activeAuction()
	withHighest.HighestBidderID = uuid.NullUUID{UUID: alice, Valid: true}
	withHighest.BidCount = 1

	upcoming := activeAuction()
	upcoming.StartTime, upcoming.EndTime = t0.Add(time.Minute), t0.Add(time.Hour)

	ended := activeAuction()
	ended.EndTime = t0

	adminClosed := activeAuction()
	adminClosed.ClosedAt = &t0

	closed := activeAuction()
	closed.StartTime, closed.EndTime = t0.Add(-72*time.Hour), t0.Add(-25*time.Hour)

	cases := []struct {
		name    string
		a       db.Auction
		bidder  uuid.UUID
		amount  int64
		wantErr string
	}{
		{"min increment is valid", activeAuction(), alice, 1050, ""},
		{"max increment is valid", activeAuction(), alice, 1500, ""},
		{"below min increment", activeAuction(), alice, 1049, "BID_OUT_OF_RANGE"},
		{"above max increment", activeAuction(), alice, 1501, "BID_OUT_OF_RANGE"},
		{"equal to current price", activeAuction(), alice, 1000, "BID_OUT_OF_RANGE"},
		{"negative amount", activeAuction(), alice, -5, "BID_OUT_OF_RANGE"},
		{"seller cannot bid", activeAuction(), seller, 1100, "SELLER_CANNOT_BID"},
		{"highest bidder cannot bid again", withHighest, alice, 1100, "ALREADY_HIGHEST_BIDDER"},
		{"other bidder can outbid", withHighest, bob, 1100, ""},
		{"not started", upcoming, alice, 1050, "AUCTION_NOT_STARTED"},
		{"ended at exactly end_time", ended, alice, 1050, "AUCTION_ENDED"},
		{"closed", closed, alice, 1050, "AUCTION_ENDED"},
		{"closed by admin while running", adminClosed, alice, 1050, "AUCTION_ENDED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBid(tc.a, tc.bidder, tc.amount, t0)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			if !apperr.IsCode(err, tc.wantErr) {
				t.Fatalf("got %v, want %s", err, tc.wantErr)
			}
		})
	}
}

func TestFirstBidUsesStartPrice(t *testing.T) {
	a := activeAuction()
	a.StartPrice, a.CurrentPrice, a.MinIncrement, a.MaxIncrement = 0, 0, 50, 50
	lo, hi := BidRange(a)
	if lo != 50 || hi != 50 {
		t.Fatalf("range = %d~%d, want 50~50", lo, hi)
	}
}

func TestExtendedEnd(t *testing.T) {
	end := t0.Add(time.Hour)
	cases := []struct {
		left     time.Duration
		extended bool
	}{
		{2 * time.Minute, false},
		{time.Minute + time.Millisecond, false},
		{time.Minute, true},
		{30 * time.Second, true},
		{time.Millisecond, true},
	}
	for _, tc := range cases {
		got, ext := ExtendedEnd(end, end.Add(-tc.left))
		if ext != tc.extended {
			t.Errorf("left=%v extended=%v want %v", tc.left, ext, tc.extended)
		}
		want := end
		if tc.extended {
			want = end.Add(2 * time.Minute)
		}
		if !got.Equal(want) {
			t.Errorf("left=%v end=%v want %v", tc.left, got, want)
		}
	}
}

func TestRepeatedExtension(t *testing.T) {
	// Every bid in the last minute extends again, so the auction keeps going
	// until a full minute passes without a valid bid.
	end := t0
	now := end.Add(-30 * time.Second)
	for i := 0; i < 3; i++ {
		var ext bool
		end, ext = ExtendedEnd(end, now)
		if !ext {
			t.Fatalf("bid %d should extend", i)
		}
		now = end.Add(-10 * time.Second)
	}
	if want := t0.Add(6 * time.Minute); !end.Equal(want) {
		t.Fatalf("end = %v, want %v", end, want)
	}
}

func TestDefaultStart(t *testing.T) {
	got := DefaultStart(time.Date(2026, 10, 21, 12, 34, 56, 0, time.UTC))
	if want := time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	got = DefaultStart(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("on the hour: got %v want %v", got, want)
	}
}

func ptr[T any](v T) *T { return &v }

func TestNormalizeDefaults(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 34, 0, 0, time.UTC)
	f, err := Input{Title: " 豹紋守宮 ", ShippingMethod: "黑貓宅急便"}.Normalize(now)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "豹紋守宮" || f.StartPrice != 0 || f.MinIncrement != 50 || f.MaxIncrement != 50 {
		t.Fatalf("unexpected defaults %+v", f)
	}
	wantStart := time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC)
	if !f.StartTime.Equal(wantStart) || !f.EndTime.Equal(wantStart.Add(48*time.Hour)) {
		t.Fatalf("times = %v ~ %v", f.StartTime, f.EndTime)
	}
}

func TestNormalizeValidation(t *testing.T) {
	now := t0
	base := func() Input {
		return Input{Title: "t", ShippingMethod: "s", StartTime: ptr(now.Add(time.Hour)), EndTime: ptr(now.Add(3 * time.Hour))}
	}
	cases := []struct {
		name string
		mut  func(*Input)
		code string
	}{
		{"empty title", func(in *Input) { in.Title = "  " }, "INVALID_TITLE"},
		{"title with newline", func(in *Input) { in.Title = "a\nb" }, "INVALID_TITLE"},
		{"title with control char", func(in *Input) { in.Title = "a\x00b" }, "INVALID_TITLE"},
		{"missing shipping method", func(in *Input) { in.ShippingMethod = "" }, "INVALID_SHIPPING_METHOD"},
		{"negative start price", func(in *Input) { in.StartPrice = ptr(int64(-1)) }, "INVALID_START_PRICE"},
		{"zero min increment", func(in *Input) { in.MinIncrement = ptr(int64(0)) }, "INVALID_MIN_INCREMENT"},
		{"max below min", func(in *Input) { in.MinIncrement, in.MaxIncrement = ptr(int64(100)), ptr(int64(50)) }, "INVALID_MAX_INCREMENT"},
		{"negative shipping fee", func(in *Input) { in.ShippingFee = -1 }, "INVALID_SHIPPING_FEE"},
		{"start in the past", func(in *Input) { in.StartTime = ptr(now.Add(-time.Hour)) }, "INVALID_START_TIME"},
		{"end before start", func(in *Input) { in.EndTime = ptr(now) }, "INVALID_END_TIME"},
		{"too short", func(in *Input) { in.EndTime = ptr(now.Add(time.Hour + 5*time.Minute)) }, "INVALID_END_TIME"},
		{"too long", func(in *Input) { in.EndTime = ptr(now.Add(15 * 24 * time.Hour)) }, "INVALID_END_TIME"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			tc.mut(&in)
			if _, err := in.Normalize(now); !apperr.IsCode(err, tc.code) {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
	if _, err := base().Normalize(now); err != nil {
		t.Fatalf("base input should be valid: %v", err)
	}
}

func TestLikePatternEscapesWildcards(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"  ":       "",
		"守宮":       "%守宮%",
		"100%":     `%100\%%`,
		"a_b":      `%a\_b%`,
		`back\sl`:  `%back\\sl%`,
		" gecko  ": "%gecko%",
	}
	for in, want := range cases {
		if got := LikePattern(in); got != want {
			t.Errorf("LikePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskName(t *testing.T) {
	cases := map[string]string{"": "***", "A": "A***", "王小": "王***", "Gecko": "G***o"}
	for in, want := range cases {
		if got := MaskName(in); got != want {
			t.Errorf("MaskName(%q) = %q, want %q", in, got, want)
		}
	}
}
