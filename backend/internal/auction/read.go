package auction

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/store"
)

const (
	PageSize        = 24
	maxPriceIDs     = 100
	detailBidsLimit = 50
	maxKeywordLen   = 50
)

type ImageDTO struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Summary is the home page card: only product info, images, current price
// and timing.
type Summary struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Images       []ImageDTO `json:"images"`
	CurrentPrice int64      `json:"current_price"`
	BidCount     int32      `json:"bid_count"`
	StartTime    time.Time  `json:"start_time"`
	EndTime      time.Time  `json:"end_time"`
	Status       Status     `json:"status"`
	SellerName   string     `json:"seller_name"`
}

type SellerDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type BidDTO struct {
	ID         string    `json:"id"`
	Amount     int64     `json:"amount"`
	BidderName string    `json:"bidder_name"`
	IsMine     bool      `json:"is_mine"`
	CreatedAt  time.Time `json:"created_at"`
}

type Detail struct {
	Summary
	Description     string    `json:"description"`
	StartPrice      int64     `json:"start_price"`
	MinIncrement    int64     `json:"min_increment"`
	MaxIncrement    int64     `json:"max_increment"`
	OriginalEndTime time.Time `json:"original_end_time"`
	ShippingMethod  string    `json:"shipping_method"`
	ShippingFee     int64     `json:"shipping_fee"`
	Seller          SellerDTO `json:"seller"`
	Bids            []BidDTO  `json:"bids"`
	NextBidMin      int64     `json:"next_bid_min"`
	NextBidMax      int64     `json:"next_bid_max"`
	IsSeller        bool      `json:"is_seller"`
	IsHighestBidder bool      `json:"is_highest_bidder"`
	// ContactInfo is only revealed to the seller (and, from Phase 3, the
	// winning buyer through the order).
	ContactInfo *string `json:"contact_info,omitempty"`
}

type Price struct {
	ID           string    `json:"id"`
	CurrentPrice int64     `json:"current_price"`
	BidCount     int32     `json:"bid_count"`
	EndTime      time.Time `json:"end_time"`
	Status       Status    `json:"status"`
}

func (s *Service) imagesFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]ImageDTO, error) {
	out := make(map[uuid.UUID][]ImageDTO, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.store.ListImagesForAuctions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	for _, r := range rows {
		out[r.AuctionID] = append(out[r.AuctionID], ImageDTO{ID: r.ID.String(), URL: s.files.URL(r.StorageKey)})
	}
	return out, nil
}

func (s *Service) summaries(ctx context.Context, auctions []db.Auction, sellerNames []string, now time.Time) ([]Summary, error) {
	ids := make([]uuid.UUID, len(auctions))
	for i, a := range auctions {
		ids[i] = a.ID
	}
	imgs, err := s.imagesFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, len(auctions))
	for i, a := range auctions {
		out[i] = summaryOf(a, sellerNames[i], imgs[a.ID], now)
	}
	return out, nil
}

func summaryOf(a db.Auction, sellerName string, imgs []ImageDTO, now time.Time) Summary {
	if imgs == nil {
		imgs = []ImageDTO{}
	}
	return Summary{
		ID: a.ID.String(), Title: a.Title, Images: imgs, CurrentPrice: a.CurrentPrice,
		BidCount: a.BidCount, StartTime: a.StartTime.UTC(), EndTime: a.EndTime.UTC(),
		Status: StatusAt(a.StartTime, a.EndTime, now), SellerName: sellerName,
	}
}

// ParseStatusFilter accepts "", UPCOMING, ACTIVE or ENDED. CLOSED auctions
// are never searchable.
func ParseStatusFilter(s string) (string, error) {
	switch Status(s) {
	case "", StatusUpcoming, StatusActive, StatusEnded:
		return s, nil
	}
	return "", apperr.Invalid("INVALID_STATUS", "狀態篩選只能為 UPCOMING、ACTIVE 或 ENDED")
}

// LikePattern turns a user keyword into an ILIKE pattern with wildcards
// escaped, so % and _ are matched literally.
func LikePattern(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return ""
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(keyword) + "%"
}

func pageOffset(page int) int32 {
	if page < 1 {
		page = 1
	}
	if page > 1000 {
		page = 1000
	}
	return int32((page - 1) * PageSize)
}

// Search lists home page auctions.
func (s *Service) Search(ctx context.Context, keyword, status string, page int) ([]Summary, time.Time, error) {
	now := s.now().UTC()
	status, err := ParseStatusFilter(status)
	if err != nil {
		return nil, now, err
	}
	if utf8.RuneCountInString(keyword) > maxKeywordLen {
		return nil, now, apperr.Invalid("KEYWORD_TOO_LONG", "關鍵字過長")
	}
	rows, err := s.store.SearchAuctions(ctx, db.SearchAuctionsParams{
		Now: now, Pattern: LikePattern(keyword), Status: status, Off: pageOffset(page), Lim: PageSize,
	})
	if err != nil {
		return nil, now, fmt.Errorf("search auctions: %w", err)
	}
	auctions := make([]db.Auction, len(rows))
	names := make([]string, len(rows))
	for i, r := range rows {
		auctions[i], names[i] = r.Auction, r.SellerName
	}
	out, err := s.summaries(ctx, auctions, names, now)
	return out, now, err
}

// ListForUser lists auctions the user is selling (role "selling") or has
// bid on (role "bidding").
func (s *Service) ListForUser(ctx context.Context, user uuid.UUID, role, status string, page int) ([]Summary, time.Time, error) {
	now := s.now().UTC()
	status, err := ParseStatusFilter(status)
	if err != nil {
		return nil, now, err
	}
	var auctions []db.Auction
	var names []string
	switch role {
	case "selling", "":
		rows, err := s.store.ListAuctionsBySeller(ctx, db.ListAuctionsBySellerParams{UserID: user, Status: status, Now: now, Off: pageOffset(page), Lim: PageSize})
		if err != nil {
			return nil, now, fmt.Errorf("list selling: %w", err)
		}
		for _, r := range rows {
			auctions, names = append(auctions, r.Auction), append(names, r.SellerName)
		}
	case "bidding":
		rows, err := s.store.ListAuctionsByBidder(ctx, db.ListAuctionsByBidderParams{UserID: user, Status: status, Now: now, Off: pageOffset(page), Lim: PageSize})
		if err != nil {
			return nil, now, fmt.Errorf("list bidding: %w", err)
		}
		for _, r := range rows {
			auctions, names = append(auctions, r.Auction), append(names, r.SellerName)
		}
	default:
		return nil, now, apperr.Invalid("INVALID_ROLE", "role 只能為 selling 或 bidding")
	}
	out, err := s.summaries(ctx, auctions, names, now)
	return out, now, err
}

// Detail returns the auction page. viewer is uuid.Nil for anonymous users.
func (s *Service) Detail(ctx context.Context, id, viewer uuid.UUID) (Detail, time.Time, error) {
	now := s.now().UTC()
	row, err := s.store.GetAuctionWithSeller(ctx, id)
	if store.IsNoRows(err) {
		return Detail{}, now, ErrNotFound
	}
	if err != nil {
		return Detail{}, now, fmt.Errorf("get auction: %w", err)
	}
	a := row.Auction
	imgs, err := s.imagesFor(ctx, []uuid.UUID{a.ID})
	if err != nil {
		return Detail{}, now, err
	}
	bids, err := s.store.ListBidsForAuction(ctx, db.ListBidsForAuctionParams{AuctionID: a.ID, Limit: detailBidsLimit})
	if err != nil {
		return Detail{}, now, fmt.Errorf("list bids: %w", err)
	}
	lo, hi := BidRange(a)
	d := Detail{
		Summary:     summaryOf(a, row.SellerName, imgs[a.ID], now),
		Description: a.Description, StartPrice: a.StartPrice, MinIncrement: a.MinIncrement,
		MaxIncrement: a.MaxIncrement, OriginalEndTime: a.OriginalEndTime.UTC(),
		ShippingMethod: a.ShippingMethod, ShippingFee: a.ShippingFee,
		Seller:     SellerDTO{ID: a.SellerID.String(), DisplayName: row.SellerName, AvatarURL: row.SellerAvatar},
		Bids:       make([]BidDTO, len(bids)),
		NextBidMin: lo, NextBidMax: hi,
		IsSeller:        viewer != uuid.Nil && viewer == a.SellerID,
		IsHighestBidder: viewer != uuid.Nil && a.HighestBidderID.Valid && a.HighestBidderID.UUID == viewer,
	}
	if d.IsSeller {
		d.ContactInfo = &a.ContactInfo
	}
	for i, b := range bids {
		mine := viewer != uuid.Nil && b.BidderID == viewer
		d.Bids[i] = BidDTO{ID: b.ID.String(), Amount: b.Amount, BidderName: MaskName(b.BidderName), IsMine: mine, CreatedAt: b.CreatedAt.UTC()}
	}
	return d, now, nil
}

// MaskName hides most of a bidder's display name: "Gecko" -> "G***o".
func MaskName(name string) string {
	r := []rune(name)
	switch len(r) {
	case 0:
		return "***"
	case 1, 2:
		return string(r[0]) + "***"
	default:
		return string(r[0]) + "***" + string(r[len(r)-1])
	}
}

// Prices returns the current price of many auctions in one query, for the
// home page's single 5-second polling request.
func (s *Service) Prices(ctx context.Context, ids []uuid.UUID) ([]Price, time.Time, error) {
	now := s.now().UTC()
	if len(ids) > maxPriceIDs {
		return nil, now, apperr.Invalid("TOO_MANY_IDS", fmt.Sprintf("一次最多查詢 %d 筆", maxPriceIDs))
	}
	if len(ids) == 0 {
		return []Price{}, now, nil
	}
	rows, err := s.store.GetAuctionPrices(ctx, ids)
	if err != nil {
		return nil, now, fmt.Errorf("get prices: %w", err)
	}
	out := make([]Price, 0, len(rows))
	for _, r := range rows {
		st := StatusAt(r.StartTime, r.EndTime, now)
		if st == StatusClosed {
			continue // CLOSED auctions disappear from the home page
		}
		out = append(out, Price{ID: r.ID.String(), CurrentPrice: r.CurrentPrice, BidCount: r.BidCount, EndTime: r.EndTime.UTC(), Status: st})
	}
	return out, now, nil
}
