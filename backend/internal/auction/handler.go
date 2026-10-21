package auction

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/auth"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/imageproc"
)

type Handler struct {
	svc       *Service
	bidLimit  *httpx.RateLimiter
	maxUpload int64
}

func NewHandler(svc *Service) *Handler {
	return &Handler{
		svc:       svc,
		bidLimit:  httpx.NewRateLimiter(1, 3), // per user
		maxUpload: MaxImages*imageproc.MaxFileBytes + 1<<20,
	}
}

// Routes mounts auction endpoints. auth.Service.LoadUser must run before.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/auctions", httpx.Handle(h.search))
	r.Get("/auctions/prices", httpx.Handle(h.prices))
	r.Get("/auctions/{id}", httpx.Handle(h.detail))
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireMember)
		r.Post("/auctions", httpx.Handle(h.create))
		r.Put("/auctions/{id}", httpx.Handle(h.update))
		r.Post("/auctions/{id}/bids", httpx.Handle(h.bid))
		r.Get("/me/auctions", httpx.Handle(h.mine))
	})
}

type listResponse struct {
	Items      []Summary `json:"items"`
	ServerTime time.Time `json:"server_time"`
}

func pageParam(r *http.Request) int {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	return p
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	items, now, err := h.svc.Search(r.Context(), q.Get("q"), q.Get("status"), pageParam(r))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, listResponse{items, now})
	return nil
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	u := auth.UserFromContext(r.Context())
	items, now, err := h.svc.ListForUser(r.Context(), u.ID, q.Get("role"), q.Get("status"), pageParam(r))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, listResponse{items, now})
	return nil
}

func (h *Handler) prices(w http.ResponseWriter, r *http.Request) error {
	raw := r.URL.Query().Get("ids")
	var ids []uuid.UUID
	if raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) > maxPriceIDs {
			return apperr.Invalid("TOO_MANY_IDS", "查詢數量過多")
		}
		for _, p := range parts {
			id, err := uuid.Parse(strings.TrimSpace(p))
			if err != nil {
				return apperr.Invalid("INVALID_ID", "競標 ID 格式錯誤")
			}
			ids = append(ids, id)
		}
	}
	items, now, err := h.svc.Prices(r.Context(), ids)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "server_time": now})
	return nil
}

func auctionID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return id, nil
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) error {
	id, err := auctionID(r)
	if err != nil {
		return err
	}
	viewer := uuid.Nil
	if u := auth.UserFromContext(r.Context()); u != nil {
		viewer = u.ID
	}
	d, now, err := h.svc.Detail(r.Context(), id, viewer)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"auction": d, "server_time": now})
	return nil
}

type writeRequest struct {
	Input
	KeepImageIDs []uuid.UUID `json:"keep_image_ids"`
}

// parseWrite reads a multipart form with a JSON "data" field and up to
// MaxImages "images" files.
func (h *Handler) parseWrite(w http.ResponseWriter, r *http.Request) (writeRequest, []Upload, error) {
	var req writeRequest
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return req, nil, apperr.Invalid("BODY_TOO_LARGE", "上傳內容過大")
		}
		return req, nil, apperr.Invalid("INVALID_FORM", "表單格式錯誤")
	}
	defer r.MultipartForm.RemoveAll()

	if err := httpx.DecodeJSONBytes(strings.NewReader(r.FormValue("data")), &req); err != nil {
		return req, nil, err
	}
	files := r.MultipartForm.File["images"]
	if len(files) > MaxImages {
		return req, nil, ErrImageCount
	}
	uploads := make([]Upload, 0, len(files))
	for _, fh := range files {
		data, err := readFile(fh)
		if err != nil {
			return req, nil, err
		}
		uploads = append(uploads, Upload{Filename: fh.Filename, Data: data})
	}
	return req, uploads, nil
}

func readFile(fh *multipart.FileHeader) ([]byte, error) {
	if fh.Size > imageproc.MaxFileBytes {
		return nil, apperr.Invalid("IMAGE_TOO_LARGE", "每張圖片不可超過 8MB")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, imageproc.MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > imageproc.MaxFileBytes {
		return nil, apperr.Invalid("IMAGE_TOO_LARGE", "每張圖片不可超過 8MB")
	}
	return data, nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	req, uploads, err := h.parseWrite(w, r)
	if err != nil {
		return err
	}
	if len(req.KeepImageIDs) > 0 {
		return apperr.Invalid("INVALID_FORM", "新增競標不可指定保留圖片")
	}
	a, err := h.svc.Create(r.Context(), auth.UserFromContext(r.Context()).ID, req.Input, uploads)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"id": a.ID.String()})
	return nil
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := auctionID(r)
	if err != nil {
		return err
	}
	req, uploads, err := h.parseWrite(w, r)
	if err != nil {
		return err
	}
	a, err := h.svc.Update(r.Context(), auth.UserFromContext(r.Context()).ID, id, req.Input, req.KeepImageIDs, uploads)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": a.ID.String()})
	return nil
}

func (h *Handler) bid(w http.ResponseWriter, r *http.Request) error {
	id, err := auctionID(r)
	if err != nil {
		return err
	}
	u := auth.UserFromContext(r.Context())
	if !h.bidLimit.Allow(u.ID.String()) {
		return apperr.TooMany("RATE_LIMITED", "出價過於頻繁，請稍後再試")
	}
	var req struct {
		Amount int64 `json:"amount"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	res, err := h.svc.PlaceBid(r.Context(), u.ID, id, req.Amount)
	if err != nil {
		return err
	}
	lo, hi := BidRange(res.Auction)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"bid_id":        res.Bid.ID.String(),
		"current_price": res.Auction.CurrentPrice,
		"bid_count":     res.Auction.BidCount,
		"end_time":      res.Auction.EndTime.UTC(),
		"extended":      res.Extended,
		"next_bid_min":  lo,
		"next_bid_max":  hi,
	})
	return nil
}
