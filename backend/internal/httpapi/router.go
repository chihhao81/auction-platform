package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
)

type errorResponse struct {
	Error        string `json:"error"`
	TermsVersion string `json:"terms_version,omitempty"`
}

func NewRouter() http.Handler {
	return NewRouterWithConfig(nil, ConfigFromEnv())
}

func NewRouterWithConfig(db *sql.DB, cfg Config) http.Handler {
	app := &App{db: db, cfg: cfg, uploadDir: cfg.UploadDir}
	if app.uploadDir == "" {
		app.uploadDir = "./uploads"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /v1/auth/line", app.startLineLogin)
	mux.HandleFunc("GET /v1/auth/line/callback", app.finishLineLogin)
	mux.HandleFunc("GET /v1/terms/current", app.currentTerms)
	mux.HandleFunc("POST /v1/terms/accept", app.acceptTerms)
	mux.HandleFunc("GET /v1/me", app.currentUser)
	mux.HandleFunc("POST /v1/logout", app.logout)
	mux.HandleFunc("GET /v1/auctions", app.listAuctions)
	mux.HandleFunc("POST /v1/auctions", app.createAuction)
	mux.HandleFunc("GET /v1/auctions/{id}", app.getAuction)
	mux.HandleFunc("PATCH /v1/auctions/{id}", app.updateAuction)
	mux.HandleFunc("POST /v1/auctions/{id}/bids", app.placeBid)
	mux.HandleFunc("POST /v1/uploads", app.uploadAuctionImage)
	mux.HandleFunc("GET /v1/uploads/{id}", app.serveAuctionImage)
	mux.HandleFunc("DELETE /v1/uploads/{id}", app.deleteAuctionImage)
	return app.cors(mux)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
