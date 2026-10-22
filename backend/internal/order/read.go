package order

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
)

const PageSize = 20

var ErrNotFound = apperr.NotFound("ORDER_NOT_FOUND", "找不到此訂單")

// Options configures the automatic order transitions.
type Options struct {
	// AutoReceiveAfter: SHIPPED orders become RECEIVED after this long.
	AutoReceiveAfter time.Duration
	// AutoCompleteAfter: RECEIVED orders become COMPLETED after this long.
	AutoCompleteAfter time.Duration
	// ShippingInfoRetention: the buyer's address is erased this long after
	// the order is completed or cancelled.
	ShippingInfoRetention time.Duration
}

// DefaultOptions holds the confirmed business defaults (see AGENTS.md).
var DefaultOptions = Options{
	AutoReceiveAfter:      3 * 24 * time.Hour,
	AutoCompleteAfter:     7 * 24 * time.Hour,
	ShippingInfoRetention: 30 * 24 * time.Hour,
}

type Service struct {
	store    *store.Store
	files    storage.Storage
	dispatch notify.Dispatcher
	opts     Options
	now      func() time.Time
}

func NewService(st *store.Store, files storage.Storage, d notify.Dispatcher, opts Options, now func() time.Time) *Service {
	return &Service{store: st, files: files, dispatch: d, opts: opts, now: now}
}

type PartyDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type ItemDTO struct {
	AuctionID      string `json:"auction_id"`
	Title          string `json:"title"`
	Amount         int64  `json:"amount"`
	ShippingFee    int64  `json:"shipping_fee"`
	ShippingMethod string `json:"shipping_method"`
	ImageURL       string `json:"image_url"`
}

type DetailDTO struct {
	ID              string     `json:"id"`
	Status          Status     `json:"status"`
	Role            string     `json:"role"` // "buyer" or "seller" from the viewer's perspective
	Buyer           PartyDTO   `json:"buyer"`
	Seller          PartyDTO   `json:"seller"`
	Items           []ItemDTO  `json:"items"`
	ItemsTotal      int64      `json:"items_total"`
	ShippingFee     int64      `json:"shipping_fee"`
	TotalAmount     int64      `json:"total_amount"`
	ContactDeadline time.Time  `json:"contact_deadline"`
	ContactedAt     *time.Time `json:"contacted_at"`
	Overdue         bool       `json:"overdue"`
	// SellerContact is the seller's extra contact info from the auctions,
	// shown only to the buyer of this order.
	SellerContact []string     `json:"seller_contact"`
	PaidAt        *time.Time   `json:"paid_at"`
	ShippedAt     *time.Time   `json:"shipped_at"`
	ReceivedAt    *time.Time   `json:"received_at"`
	CompletedAt   *time.Time   `json:"completed_at"`
	Shipment      *ShipmentDTO `json:"shipment"`
	// MyReview / TheirReview: the viewer's review of the other party and
	// the other party's review of the viewer (null until written).
	MyReview    *OrderReviewDTO `json:"my_review"`
	TheirReview *OrderReviewDTO `json:"their_review"`
	// AutoAdvanceAt is when the scheduler will move a SHIPPED / RECEIVED
	// order forward if the buyer does nothing.
	AutoAdvanceAt *time.Time `json:"auto_advance_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type OrderReviewDTO struct {
	Rating    string    `json:"rating"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// ShipmentDTO is visible only to the order's buyer and seller.
type ShipmentDTO struct {
	RecipientName    string     `json:"recipient_name"`
	RecipientPhone   string     `json:"recipient_phone"`
	RecipientAddress string     `json:"recipient_address"`
	Carrier          string     `json:"carrier"`
	TrackingNumber   string     `json:"tracking_number"`
	ShippedAt        *time.Time `json:"shipped_at"`
	Purged           bool       `json:"purged"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type SummaryDTO struct {
	ID               string    `json:"id"`
	Status           Status    `json:"status"`
	TotalAmount      int64     `json:"total_amount"`
	ItemCount        int64     `json:"item_count"`
	CounterpartyName string    `json:"counterparty_name"`
	Overdue          bool      `json:"overdue"`
	CreatedAt        time.Time `json:"created_at"`
}

// Get returns an order visible to viewer. Orders of other users are reported
// as not found so their existence is not disclosed (IDOR protection).
func (s *Service) Get(ctx context.Context, id, viewer uuid.UUID) (DetailDTO, error) {
	o, err := s.store.GetOrder(ctx, id)
	if store.IsNoRows(err) {
		return DetailDTO{}, ErrNotFound
	}
	if err != nil {
		return DetailDTO{}, fmt.Errorf("get order: %w", err)
	}
	var role string
	switch viewer {
	case o.BuyerID:
		role = "buyer"
	case o.SellerID:
		role = "seller"
	default:
		return DetailDTO{}, ErrNotFound
	}
	buyer, err := s.store.GetUserPublic(ctx, o.BuyerID)
	if err != nil {
		return DetailDTO{}, fmt.Errorf("get buyer: %w", err)
	}
	seller, err := s.store.GetUserPublic(ctx, o.SellerID)
	if err != nil {
		return DetailDTO{}, fmt.Errorf("get seller: %w", err)
	}
	rows, err := s.store.ListOrderItems(ctx, uuid.NullUUID{UUID: o.ID, Valid: true})
	if err != nil {
		return DetailDTO{}, fmt.Errorf("list items: %w", err)
	}
	d := DetailDTO{
		ID: o.ID.String(), Status: Status(o.Status), Role: role,
		Buyer:  PartyDTO{buyer.ID.String(), buyer.DisplayName, buyer.AvatarUrl},
		Seller: PartyDTO{seller.ID.String(), seller.DisplayName, seller.AvatarUrl},
		Items:  make([]ItemDTO, len(rows)), ItemsTotal: o.ItemsTotal, ShippingFee: o.ShippingFee,
		TotalAmount: o.TotalAmount, ContactDeadline: o.ContactDeadline.UTC(), ContactedAt: o.ContactedAt,
		Overdue: o.OverdueAt != nil, SellerContact: []string{}, CreatedAt: o.CreatedAt.UTC(),
		PaidAt: o.PaidAt, ShippedAt: o.ShippedAt, ReceivedAt: o.ReceivedAt, CompletedAt: o.CompletedAt,
	}
	switch {
	case Status(o.Status) == Shipped && o.ShippedAt != nil:
		d.AutoAdvanceAt = ptr(o.ShippedAt.Add(s.opts.AutoReceiveAfter).UTC())
	case Status(o.Status) == Received && o.ReceivedAt != nil:
		d.AutoAdvanceAt = ptr(o.ReceivedAt.Add(s.opts.AutoCompleteAfter).UTC())
	}
	revs, err := s.store.ListOrderReviews(ctx, o.ID)
	if err != nil {
		return DetailDTO{}, fmt.Errorf("list reviews: %w", err)
	}
	for _, r := range revs {
		dto := &OrderReviewDTO{Rating: r.Rating, Comment: r.Comment, CreatedAt: r.CreatedAt.UTC()}
		if r.ReviewerID == viewer {
			d.MyReview = dto
		} else {
			d.TheirReview = dto
		}
	}
	sh, err := s.store.GetShipment(ctx, o.ID)
	switch {
	case err == nil:
		d.Shipment = &ShipmentDTO{
			RecipientName: deref(sh.RecipientName), RecipientPhone: deref(sh.RecipientPhone),
			RecipientAddress: deref(sh.RecipientAddress), Carrier: deref(sh.Carrier),
			TrackingNumber: deref(sh.TrackingNumber), ShippedAt: sh.ShippedAt, Purged: sh.PurgedAt != nil,
		}
	case !store.IsNoRows(err):
		return DetailDTO{}, fmt.Errorf("get shipment: %w", err)
	}
	seen := map[string]bool{}
	for i, it := range rows {
		img := ""
		if it.CoverKey != "" {
			img = s.files.URL(it.CoverKey)
		}
		d.Items[i] = ItemDTO{
			AuctionID: it.AuctionID.String(), Title: it.Title, Amount: it.Amount,
			ShippingFee: it.ShippingFee, ShippingMethod: it.ShippingMethod, ImageURL: img,
		}
		if c := strings.TrimSpace(it.ContactInfo); role == "buyer" && c != "" && !seen[c] {
			seen[c] = true
			d.SellerContact = append(d.SellerContact, c)
		}
	}
	return d, nil
}

// List returns the viewer's orders as buyer ("buyer") or seller ("seller").
func (s *Service) List(ctx context.Context, viewer uuid.UUID, role string, page int) ([]SummaryDTO, error) {
	if role == "" {
		role = "buyer"
	}
	if role != "buyer" && role != "seller" {
		return nil, apperr.Invalid("INVALID_ROLE", "role 只能為 buyer 或 seller")
	}
	if page < 1 || page > 1000 {
		page = 1
	}
	rows, err := s.store.ListOrdersForUser(ctx, db.ListOrdersForUserParams{
		Role: role, UserID: viewer, Lim: PageSize, Off: int32((page - 1) * PageSize),
	})
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	out := make([]SummaryDTO, len(rows))
	for i, r := range rows {
		out[i] = SummaryDTO{
			ID: r.ID.String(), Status: Status(r.Status), TotalAmount: r.TotalAmount, ItemCount: r.ItemCount,
			CounterpartyName: r.CounterpartyName, Overdue: r.OverdueAt != nil, CreatedAt: r.CreatedAt.UTC(),
		}
	}
	return out, nil
}
