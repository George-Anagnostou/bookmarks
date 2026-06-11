package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"bookmarks/internal/bookmarks"
)

type Config struct {
	Store bookmarks.Store
	Token string
}

type Server struct {
	store bookmarks.Store
	token string
}

type createBookmarkResponse struct {
	Bookmark bookmarks.Bookmark `json:"bookmark"`
	Created  bool               `json:"created"`
}

// create a new server
// panic if Config is not set correctly to prevent bad startups
func New(cfg Config) *Server {
	if cfg.Store == nil {
		panic("server store is required")
	}
	if cfg.Token == "" {
		panic("server token is required")
	}

	return &Server{
		store: cfg.Store,
		token: cfg.Token,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/bookmarks", s.requireAuth(s.handleCreateBookmark))
	// mux.HandleFunc("GET /api/bookmarks", s.requireAuth(s.handleListBookmarksJSON))
	// mux.HandleFunc("GET /{$}", s.requireAuth(s.handleIndex))
	return mux
}

func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var input bookmarks.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	bookmark, created, err := s.store.CreateBookmark(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, bookmarks.ErrEmptyURL),
			errors.Is(err, bookmarks.ErrUnsupported),
			errors.Is(err, bookmarks.ErrMissingHost),
			errors.Is(err, bookmarks.ErrURLUserInfo):
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(createBookmarkResponse{
		Bookmark: bookmark,
		Created:  created,
	}); err != nil {
		return
	}
}

func (s *Server) handleListBookmarksJSON(w http.ResponseWriter, r *http.Request) {}
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request)             {}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got, ok := bearerToken(r)
		if !ok || !constantTimeEqual(got, s.token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func bearerToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

// using constant-time avoids leaking partial info about token through tiny timing differneces
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
