package httpapi

import (
	"bytes"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
	"time"
)

func TestAuctionStatusBoundaries(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		start, end time.Time
		want       string
	}{
		{name: "upcoming", start: now.Add(time.Second), end: now.Add(time.Hour), want: "UPCOMING"},
		{name: "active at start", start: now, end: now.Add(time.Hour), want: "ACTIVE"},
		{name: "active before end", start: now.Add(-time.Hour), end: now.Add(time.Nanosecond), want: "ACTIVE"},
		{name: "ended at end", start: now.Add(-time.Hour), end: now, want: "ENDED"},
		{name: "closed at 24 hours", start: now.Add(-25 * time.Hour), end: now.Add(-24 * time.Hour), want: "CLOSED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auctionStatus(tc.start, tc.end, now); got != tc.want {
				t.Fatalf("auctionStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextLegalBid(t *testing.T) {
	if err := nextLegalBid(1000, 50, 500, 1050); err != nil {
		t.Fatalf("minimum bid rejected: %v", err)
	}
	if err := nextLegalBid(1000, 50, 500, 1500); err != nil {
		t.Fatalf("maximum bid rejected: %v", err)
	}
	for _, amount := range []int64{1049, 1501, 0} {
		if err := nextLegalBid(1000, 50, 500, amount); err == nil {
			t.Errorf("amount %d should be rejected", amount)
		}
	}
	if err := nextLegalBid(math.MaxInt64-2, 1, 5, math.MaxInt64); err == nil {
		t.Fatal("overflowing price range should be rejected")
	}
}

func TestValidateBidSellerHighestBidderAndAuctionState(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	start, end := now.Add(-time.Hour), now.Add(time.Minute)
	cases := []struct {
		name            string
		seller, bidder  int64
		highest         sql.NullInt64
		start, end, now time.Time
		amount          int64
		wantErr         bool
	}{
		{name: "valid bid", seller: 1, bidder: 2, highest: sql.NullInt64{}, start: start, end: end, now: now, amount: 1050},
		{name: "seller cannot bid", seller: 1, bidder: 1, start: start, end: end, now: now, amount: 1050, wantErr: true},
		{name: "highest bidder cannot bid again", seller: 1, bidder: 2, highest: sql.NullInt64{Int64: 2, Valid: true}, start: start, end: end, now: now, amount: 1050, wantErr: true},
		{name: "not started", seller: 1, bidder: 2, start: now.Add(time.Second), end: end, now: now, amount: 1050, wantErr: true},
		{name: "ended", seller: 1, bidder: 2, start: start, end: now, now: now, amount: 1050, wantErr: true},
		{name: "invalid increment", seller: 1, bidder: 2, start: start, end: end, now: now, amount: 1049, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBid(tc.seller, tc.bidder, tc.highest, tc.start, tc.end, tc.now, 1000, 50, 500, tc.amount)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateBid() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestExtendedEndTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		remaining time.Duration
		want      time.Duration
	}{
		{name: "one minute extends", remaining: time.Minute, want: 3 * time.Minute},
		{name: "inside one minute extends", remaining: 30 * time.Second, want: 2*time.Minute + 30*time.Second},
		{name: "outside one minute stays", remaining: time.Minute + time.Nanosecond, want: time.Minute + time.Nanosecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			end := now.Add(tc.remaining)
			got := extendedEndTime(end, now).Sub(now)
			if got != tc.want {
				t.Fatalf("extended duration = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateAuctionInput(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	valid := auctionInput{Title: "守宮", Description: "健康個體", MinIncrement: 50, MaxIncrement: 500, StartsAt: now.Add(time.Hour), EndsAt: now.Add(49 * time.Hour), Images: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	if err := validateAuctionInput(valid, now, true); err != nil {
		t.Fatalf("valid auction rejected: %v", err)
	}
	invalid := valid
	invalid.Images = []string{"../../private"}
	if err := validateAuctionInput(invalid, now, true); err == nil {
		t.Fatal("an arbitrary image key should be rejected")
	}
	invalid = valid
	invalid.EndsAt = invalid.StartsAt
	if err := validateAuctionInput(invalid, now, true); err == nil {
		t.Fatal("auction with end before or at start should be rejected")
	}
	invalid = valid
	invalid.StartsAt = now.Add(-time.Second)
	if err := validateAuctionInput(invalid, now, true); err == nil {
		t.Fatal("new auction starting in the past should be rejected")
	}
	invalid = valid
	invalid.Images = []string{valid.Images[0], valid.Images[0]}
	if err := validateAuctionInput(invalid, now, true); err == nil {
		t.Fatal("duplicate uploaded image ID should be rejected")
	}
}

func TestImageFormatRequiresAllowedExtension(t *testing.T) {
	for _, tc := range []struct {
		name, format string
		valid        bool
	}{
		{name: "animal.JPG", format: "jpeg", valid: true}, {name: "image.jpeg", format: "jpeg", valid: true}, {name: "image.png", format: "png", valid: true}, {name: "image.gif", format: "gif", valid: true}, {name: "image.svg", valid: false}, {name: "image.exe", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := imageFormatForExtension(tc.name)
			if ok != tc.valid || (ok && got != tc.format) {
				t.Fatalf("imageFormatForExtension()=(%q,%v), want (%q,%v)", got, ok, tc.format, tc.valid)
			}
		})
	}
}

func TestInspectAuctionImageChecksContentAndExtension(t *testing.T) {
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 20, G: 80, B: 30, A: 255})
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	content := buffer.Bytes()
	if got, err := inspectAuctionImage("pet.png", content); err != nil || got != "image/png" {
		t.Fatalf("valid PNG rejected: mime=%q err=%v", got, err)
	}
	if _, err := inspectAuctionImage("pet.jpg", content); err == nil {
		t.Fatal("mismatched extension accepted")
	}
	if _, err := inspectAuctionImage("pet.png", []byte("not an image")); err == nil {
		t.Fatal("invalid image content accepted")
	}
	if _, err := inspectAuctionImage("pet.png", bytes.Repeat([]byte{0}, maxAuctionImageBytes+1)); err == nil {
		t.Fatal("oversized image accepted")
	}
}
