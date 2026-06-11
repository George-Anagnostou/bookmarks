package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
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

type errorResponse struct {
	Error string `json:"error"`
}

// Cap response bodies at 64KB
const maxCreateBookmarkBodyBytes = 64 * 1024

// New creates a server and panics if required config is missing.
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
	mux.HandleFunc("GET /api/bookmarks", s.requireAuth(s.handleListBookmarksJSON))
	// mux.HandleFunc("GET /{$}", s.requireAuth(s.handleIndex))
	return mux
}

func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var input bookmarks.CreateInput
	if err := decodeJSON(w, r, &input, maxCreateBookmarkBodyBytes); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "bad request"})
		return
	}

	bookmark, created, err := s.store.CreateBookmark(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, bookmarks.ErrEmptyURL),
			errors.Is(err, bookmarks.ErrUnsupported),
			errors.Is(err, bookmarks.ErrMissingHost),
			errors.Is(err, bookmarks.ErrURLUserInfo):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "bad request"})
			return
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
			return
		}
	}

	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}

	writeJSON(w, status, createBookmarkResponse{
		Bookmark: bookmark,
		Created:  created,
	})
}

func (s *Server) handleListBookmarksJSON(w http.ResponseWriter, r *http.Request) {}
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request)             {}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got, ok := bearerToken(r)
		if !ok || !constantTimeEqual(got, s.token) {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
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

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// using constant-time avoids leaking partial info about token through tiny timing differences
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
