package httpapi

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxAuctionRequestBytes = 1 << 20

var (
	errAuctionNotActive       = errors.New("auction is not active")
	errSellerCannotBid        = errors.New("seller cannot bid on their own auction")
	errHighestBidderCannotBid = errors.New("current highest bidder cannot bid again")
	errImagesUnavailable      = errors.New("one or more images are unavailable")
)

type auctionInput struct {
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	StartingPrice  int64     `json:"starting_price"`
	MinIncrement   int64     `json:"min_increment"`
	MaxIncrement   int64     `json:"max_increment"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	ContactMethod  string    `json:"contact_method"`
	ShippingMethod string    `json:"shipping_method"`
	ShippingFee    int64     `json:"shipping_fee"`
	Images         []string  `json:"images"`
}

type auctionResponse struct {
	ID              int64     `json:"id"`
	SellerID        int64     `json:"seller_id"`
	SellerName      string    `json:"seller_name"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	StartingPrice   int64     `json:"starting_price"`
	CurrentPrice    int64     `json:"current_price"`
	MinIncrement    int64     `json:"min_increment"`
	MaxIncrement    int64     `json:"max_increment"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	Status          string    `json:"status"`
	BidCount        int64     `json:"bid_count"`
	HighestBidderID *int64    `json:"highest_bidder_id,omitempty"`
	ContactMethod   string    `json:"contact_method,omitempty"`
	ShippingMethod  string    `json:"shipping_method"`
	ShippingFee     int64     `json:"shipping_fee"`
	Images          []string  `json:"images"`
	CreatedAt       time.Time `json:"created_at"`
}

type bidResponse struct {
	ID         int64     `json:"id"`
	BidderID   int64     `json:"bidder_id"`
	BidderName string    `json:"bidder_name"`
	Amount     int64     `json:"amount"`
	CreatedAt  time.Time `json:"created_at"`
}

func auctionStatus(startsAt, endsAt, now time.Time) string {
	switch {
	case now.Before(startsAt):
		return "UPCOMING"
	case now.Before(endsAt):
		return "ACTIVE"
	case now.Before(endsAt.Add(24 * time.Hour)):
		return "ENDED"
	default:
		return "CLOSED"
	}
}

func validateAuctionInput(input auctionInput, now time.Time, creating bool) error {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if len([]rune(input.Title)) < 1 || len([]rune(input.Title)) > 120 {
		return errors.New("title must contain 1 to 120 characters")
	}
	if len([]rune(input.Description)) < 1 || len([]rune(input.Description)) > 10000 {
		return errors.New("description must contain 1 to 10000 characters")
	}
	if input.StartingPrice < 0 || input.ShippingFee < 0 {
		return errors.New("prices and shipping fee cannot be negative")
	}
	if input.MinIncrement <= 0 || input.MaxIncrement < input.MinIncrement {
		return errors.New("max_increment must be greater than or equal to a positive min_increment")
	}
	if input.StartsAt.IsZero() || input.EndsAt.IsZero() || !input.EndsAt.After(input.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	if creating && input.StartsAt.Before(now) {
		return errors.New("starts_at cannot be in the past")
	}
	if len([]rune(input.ContactMethod)) > 500 || len([]rune(input.ShippingMethod)) > 500 {
		return errors.New("contact_method and shipping_method must be at most 500 characters")
	}
	if len(input.Images) > 4 {
		return errors.New("an auction can have at most 4 images")
	}
	seenImages := make(map[string]struct{}, len(input.Images))
	for _, image := range input.Images {
		if !validUploadID(image) {
			return errors.New("images must be uploaded image IDs")
		}
		if _, exists := seenImages[image]; exists {
			return errors.New("duplicate image IDs are not allowed")
		}
		seenImages[image] = struct{}{}
	}
	return nil
}

func validUploadID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func nextLegalBid(current, min, max, amount int64) error {
	if current > math.MaxInt64-max {
		return errors.New("price limit reached")
	}
	if amount < current+min || amount > current+max {
		return errors.New("bid must be within the allowed increment range")
	}
	return nil
}

func validateBid(sellerID, bidderID int64, highestBidder sql.NullInt64, startsAt, endsAt, now time.Time, current, min, max, amount int64) error {
	if !now.Before(endsAt) || now.Before(startsAt) {
		return errAuctionNotActive
	}
	if sellerID == bidderID {
		return errSellerCannotBid
	}
	if highestBidder.Valid && highestBidder.Int64 == bidderID {
		return errHighestBidderCannotBid
	}
	return nextLegalBid(current, min, max, amount)
}

func extendedEndTime(endsAt, now time.Time) time.Time {
	if endsAt.Sub(now) <= time.Minute {
		return endsAt.Add(2 * time.Minute)
	}
	return endsAt
}

func (a *App) requireAuctionUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return 0, false
	}
	userID, err := a.sessionUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "login required")
		return 0, false
	}
	var status string
	err = a.db.QueryRowContext(r.Context(), `SELECT status FROM users WHERE id = $1`, userID).Scan(&status)
	if err != nil || status != "ACTIVE" {
		writeError(w, http.StatusForbidden, "account is not active")
		return 0, false
	}
	var accepted bool
	err = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_terms WHERE user_id=$1 AND terms_version=$2)`, userID, a.cfg.TermsVersion).Scan(&accepted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check terms acceptance")
		return 0, false
	}
	if !accepted {
		writeJSON(w, http.StatusPreconditionRequired, errorResponse{Error: "current terms must be accepted", TermsVersion: a.cfg.TermsVersion})
		return 0, false
	}
	return userID, true
}

func (a *App) listAuctions(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "CLOSED" {
		writeError(w, http.StatusBadRequest, "closed auctions cannot be searched")
		return
	}
	if status != "" && status != "UPCOMING" && status != "ACTIVE" && status != "ENDED" {
		writeError(w, http.StatusBadRequest, "invalid auction status")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 100 {
		writeError(w, http.StatusBadRequest, "search query is too long")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT a.id, a.seller_id, u.display_name, a.title, a.description,
		       a.starting_price, a.current_price, a.min_increment, a.max_increment,
		       a.starts_at, a.ends_at, a.highest_bidder_id, a.shipping_method,
	       a.shipping_fee, a.created_at,
	       (SELECT count(*) FROM bids b WHERE b.auction_id=a.id),
	       (SELECT COALESCE(json_agg('/v1/uploads/' || ai.upload_id ORDER BY ai.position), '[]'::json)::text FROM auction_images ai WHERE ai.auction_id=a.id),
		       CASE WHEN now() < a.starts_at THEN 'UPCOMING'
		            WHEN now() < a.ends_at THEN 'ACTIVE'
		            WHEN now() < a.ends_at + interval '24 hours' THEN 'ENDED'
		            ELSE 'CLOSED' END AS status
		FROM auctions a JOIN users u ON u.id=a.seller_id
		WHERE now() < a.ends_at + interval '24 hours'
		  AND ($1='' OR a.title ILIKE '%' || $1 || '%' OR a.description ILIKE '%' || $1 || '%')
		  AND ($2='' OR CASE WHEN now() < a.starts_at THEN 'UPCOMING'
		                     WHEN now() < a.ends_at THEN 'ACTIVE' ELSE 'ENDED' END = $2)
		ORDER BY a.starts_at ASC, a.id DESC LIMIT 100`, query, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list auctions")
		return
	}
	defer rows.Close()
	items := make([]auctionResponse, 0)
	for rows.Next() {
		var item auctionResponse
		var imagesJSON string
		if err := rows.Scan(&item.ID, &item.SellerID, &item.SellerName, &item.Title, &item.Description,
			&item.StartingPrice, &item.CurrentPrice, &item.MinIncrement, &item.MaxIncrement,
			&item.StartsAt, &item.EndsAt, &item.HighestBidderID, &item.ShippingMethod,
			&item.ShippingFee, &item.CreatedAt, &item.BidCount, &imagesJSON, &item.Status); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read auctions")
			return
		}
		if err := json.Unmarshal([]byte(imagesJSON), &item.Images); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read auction images")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read auctions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auctions": items})
}

func (a *App) getAuction(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid auction id")
		return
	}
	var item auctionResponse
	var status string
	var contactMethod string
	err = a.db.QueryRowContext(r.Context(), `
		SELECT a.id, a.seller_id, u.display_name, a.title, a.description,
		       a.starting_price, a.current_price, a.min_increment, a.max_increment,
		       a.starts_at, a.ends_at, a.highest_bidder_id, a.contact_method,
		       a.shipping_method, a.shipping_fee, a.created_at,
		       CASE WHEN now() < a.starts_at THEN 'UPCOMING'
		            WHEN now() < a.ends_at THEN 'ACTIVE'
		            WHEN now() < a.ends_at + interval '24 hours' THEN 'ENDED'
		            ELSE 'CLOSED' END,
		       (SELECT count(*) FROM bids WHERE auction_id=a.id)
		FROM auctions a JOIN users u ON u.id=a.seller_id WHERE a.id=$1`, id).
		Scan(&item.ID, &item.SellerID, &item.SellerName, &item.Title, &item.Description,
			&item.StartingPrice, &item.CurrentPrice, &item.MinIncrement, &item.MaxIncrement,
			&item.StartsAt, &item.EndsAt, &item.HighestBidderID, &contactMethod,
			&item.ShippingMethod, &item.ShippingFee, &item.CreatedAt, &status, &item.BidCount)
	if err == sql.ErrNoRows || (err == nil && status == "CLOSED") {
		writeError(w, http.StatusNotFound, "auction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load auction")
		return
	}
	item.Status = status
	if viewerID, err := a.sessionUserID(r); err == nil && viewerID == item.SellerID {
		var canViewContact bool
		if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM users u JOIN user_terms t ON t.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND t.terms_version=$2)`, viewerID, a.cfg.TermsVersion).Scan(&canViewContact); err == nil && canViewContact {
			item.ContactMethod = contactMethod
		}
	}
	item.Images = []string{}
	imageRows, err := a.db.QueryContext(r.Context(), `SELECT '/v1/uploads/' || upload_id FROM auction_images WHERE auction_id=$1 ORDER BY position`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load auction images")
		return
	}
	for imageRows.Next() {
		var image string
		if err := imageRows.Scan(&image); err != nil {
			imageRows.Close()
			writeError(w, http.StatusInternalServerError, "could not load auction images")
			return
		}
		item.Images = append(item.Images, image)
	}
	if err := imageRows.Err(); err != nil {
		imageRows.Close()
		writeError(w, http.StatusInternalServerError, "could not load auction images")
		return
	}
	imageRows.Close()
	bidRows, err := a.db.QueryContext(r.Context(), `SELECT b.id, b.bidder_id, u.display_name, b.amount, b.created_at FROM bids b JOIN users u ON u.id=b.bidder_id WHERE b.auction_id=$1 ORDER BY b.created_at DESC, b.id DESC LIMIT 50`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load bids")
		return
	}
	bids := make([]bidResponse, 0)
	for bidRows.Next() {
		var bid bidResponse
		if err := bidRows.Scan(&bid.ID, &bid.BidderID, &bid.BidderName, &bid.Amount, &bid.CreatedAt); err != nil {
			bidRows.Close()
			writeError(w, http.StatusInternalServerError, "could not load bids")
			return
		}
		bids = append(bids, bid)
	}
	if err := bidRows.Err(); err != nil {
		bidRows.Close()
		writeError(w, http.StatusInternalServerError, "could not load bids")
		return
	}
	bidRows.Close()
	writeJSON(w, http.StatusOK, map[string]any{"auction": item, "bids": bids})
}

func decodeAuctionInput(w http.ResponseWriter, r *http.Request) (auctionInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuctionRequestBytes)
	var input auctionInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid auction payload")
		return auctionInput{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid auction payload")
		return auctionInput{}, false
	}
	return input, true
}

func (a *App) createAuction(w http.ResponseWriter, r *http.Request) {
	if !a.validOrigin(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	sellerID, ok := a.requireAuctionUser(w, r)
	if !ok {
		return
	}
	input, ok := decodeAuctionInput(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	if input.MinIncrement == 0 {
		input.MinIncrement = 50
	}
	if input.MaxIncrement == 0 {
		input.MaxIncrement = 50
	}
	if input.StartsAt.IsZero() {
		input.StartsAt = now.Truncate(time.Hour).Add(time.Hour)
	}
	if input.EndsAt.IsZero() {
		input.EndsAt = input.StartsAt.Add(48 * time.Hour)
	}
	if err := validateAuctionInput(input, now, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create auction")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock($1)`, sellerID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create auction")
		return
	}
	var count int
	err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM auctions WHERE seller_id=$1 AND (starts_at>statement_timestamp() OR (starts_at<=statement_timestamp() AND ends_at>statement_timestamp()))`, sellerID).Scan(&count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check active auctions")
		return
	}
	if count >= 5 {
		writeError(w, http.StatusConflict, "you can have at most 5 upcoming or active auctions")
		return
	}
	var id int64
	err = tx.QueryRowContext(r.Context(), `INSERT INTO auctions (seller_id,title,description,starting_price,current_price,min_increment,max_increment,starts_at,ends_at,contact_method,shipping_method,shipping_fee) VALUES ($1,$2,$3,$4,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, sellerID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), input.StartingPrice, input.MinIncrement, input.MaxIncrement, input.StartsAt.UTC(), input.EndsAt.UTC(), strings.TrimSpace(input.ContactMethod), strings.TrimSpace(input.ShippingMethod), input.ShippingFee).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create auction")
		return
	}
	if err := replaceAuctionImages(r.Context(), tx, id, sellerID, input.Images); err != nil {
		if errors.Is(err, errImagesUnavailable) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not save auction images")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create auction")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func replaceAuctionImages(ctx context.Context, tx *sql.Tx, auctionID, ownerID int64, images []string) error {
	for _, imageID := range images {
		var imageOwner int64
		var currentAuction sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT owner_id, auction_id FROM auction_image_uploads WHERE id=$1 FOR UPDATE`, imageID).Scan(&imageOwner, &currentAuction)
		if errors.Is(err, sql.ErrNoRows) {
			return errImagesUnavailable
		}
		if err != nil {
			return err
		}
		if imageOwner != ownerID || (currentAuction.Valid && currentAuction.Int64 != auctionID) {
			return errImagesUnavailable
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auction_images WHERE auction_id=$1`, auctionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auction_image_uploads SET auction_id=NULL WHERE auction_id=$1`, auctionID); err != nil {
		return err
	}
	for position, imageID := range images {
		if _, err := tx.ExecContext(ctx, `UPDATE auction_image_uploads SET auction_id=$1 WHERE id=$2`, auctionID, imageID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO auction_images (auction_id,position,upload_id) VALUES ($1,$2,$3)`, auctionID, position, imageID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) updateAuction(w http.ResponseWriter, r *http.Request) {
	if !a.validOrigin(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	sellerID, ok := a.requireAuctionUser(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid auction id")
		return
	}
	input, ok := decodeAuctionInput(w, r)
	if !ok {
		return
	}
	if input.MinIncrement == 0 {
		input.MinIncrement = 50
	}
	if input.MaxIncrement == 0 {
		input.MaxIncrement = 50
	}
	if err := validateAuctionInput(input, time.Now().UTC(), false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update auction")
		return
	}
	defer tx.Rollback()
	var owner int64
	var starts time.Time
	err = tx.QueryRowContext(r.Context(), `SELECT seller_id, starts_at FROM auctions WHERE id=$1 FOR UPDATE`, id).Scan(&owner, &starts)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "auction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update auction")
		return
	}
	if owner != sellerID {
		writeError(w, http.StatusForbidden, "only the seller can edit this auction")
		return
	}
	if !starts.After(time.Now().UTC()) || !input.StartsAt.After(time.Now().UTC()) {
		writeError(w, http.StatusConflict, "only upcoming auctions can be edited")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE auctions SET title=$1, description=$2, starting_price=$3, current_price=$3, min_increment=$4, max_increment=$5, starts_at=$6, ends_at=$7, contact_method=$8, shipping_method=$9, shipping_fee=$10, updated_at=now() WHERE id=$11`, strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), input.StartingPrice, input.MinIncrement, input.MaxIncrement, input.StartsAt.UTC(), input.EndsAt.UTC(), strings.TrimSpace(input.ContactMethod), strings.TrimSpace(input.ShippingMethod), input.ShippingFee, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update auction")
		return
	}
	if err = replaceAuctionImages(r.Context(), tx, id, sellerID, input.Images); err != nil {
		if errors.Is(err, errImagesUnavailable) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not update auction images")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update auction")
		return
	}
	r.SetPathValue("id", strconv.FormatInt(id, 10))
	a.getAuction(w, r)
}

type bidInput struct {
	Amount int64 `json:"amount"`
}

func (a *App) placeBid(w http.ResponseWriter, r *http.Request) {
	if !a.validOrigin(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	userID, ok := a.requireAuctionUser(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid auction id")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input bidInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid bid payload")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid bid payload")
		return
	}
	if input.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not place bid")
		return
	}
	defer tx.Rollback()
	var sellerID int64
	var current, min, max int64
	var starts, ends time.Time
	var highest sql.NullInt64
	err = tx.QueryRowContext(r.Context(), `SELECT seller_id,current_price,min_increment,max_increment,starts_at,ends_at,highest_bidder_id FROM auctions WHERE id=$1 FOR UPDATE`, id).Scan(&sellerID, &current, &min, &max, &starts, &ends, &highest)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "auction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not place bid")
		return
	}
	now := time.Now().UTC()
	if !now.Before(ends.Add(24 * time.Hour)) {
		writeError(w, http.StatusNotFound, "auction not found")
		return
	}
	if err := validateBid(sellerID, userID, highest, starts, ends, now, current, min, max, input.Amount); err != nil {
		if errors.Is(err, errSellerCannotBid) {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if errors.Is(err, errAuctionNotActive) || errors.Is(err, errHighestBidderCannotBid) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO bids (auction_id,bidder_id,amount,created_at) VALUES ($1,$2,$3,$4)`, id, userID, input.Amount, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not place bid")
		return
	}
	newEnd := extendedEndTime(ends, now)
	_, err = tx.ExecContext(r.Context(), `UPDATE auctions SET current_price=$1, highest_bidder_id=$2, ends_at=$3, updated_at=$4 WHERE id=$5`, input.Amount, userID, newEnd, now, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update auction")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not place bid")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"auction_id": id, "current_price": input.Amount, "ends_at": newEnd})
}
