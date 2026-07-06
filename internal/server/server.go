package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
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

type listBookmarksResponse struct {
	Bookmarks []bookmarks.Bookmark `json:"bookmarks"`
}

type updateBookmarkResponse struct {
	Bookmark bookmarks.Bookmark `json:"bookmark"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Cap request bodies at 64KB
const maxBookmarkBodyBytes = 64 * 1024

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
	mux.HandleFunc("PATCH /api/bookmarks/{id}", s.requireAuth(s.handleUpdateBookmark))
	mux.HandleFunc("DELETE /api/bookmarks/{id}", s.requireAuth(s.handleDeleteBookmark))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var input bookmarks.CreateInput
	if err := decodeJSON(w, r, &input, maxBookmarkBodyBytes); err != nil {
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

func (s *Server) handleListBookmarksJSON(w http.ResponseWriter, r *http.Request) {
	listQuery, err := getListQuery(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid parameters"})
		return
	}

	bookmarksList, err := s.store.ListBookmarks(r.Context(), listQuery)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}

	if bookmarksList == nil {
		bookmarksList = []bookmarks.Bookmark{}
	}

	writeJSON(w, http.StatusOK, listBookmarksResponse{
		Bookmarks: bookmarksList,
	})
}

func (s *Server) handleUpdateBookmark(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input bookmarks.UpdateInput
	if err := decodeJSON(w, r, &input, maxBookmarkBodyBytes); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "bad request"})
		return
	}

	updatedBookmark, err := s.store.UpdateBookmark(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, bookmarks.ErrEmptyURL),
			errors.Is(err, bookmarks.ErrUnsupported),
			errors.Is(err, bookmarks.ErrMissingHost),
			errors.Is(err, bookmarks.ErrURLUserInfo),
			errors.Is(err, bookmarks.ErrNoUpdateFields):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "bad request"})
			return
		case errors.Is(err, bookmarks.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		case errors.Is(err, bookmarks.ErrDuplicateURL):
			writeJSON(w, http.StatusConflict, errorResponse{Error: "conflict"})
			return
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
			return
		}
	}

	writeJSON(w, http.StatusOK, updateBookmarkResponse{
		Bookmark: updatedBookmark,
	})
}

func (s *Server) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.store.DeleteBookmark(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, bookmarks.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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

func getListQuery(r *http.Request) (bookmarks.ListQuery, error) {
	query := r.URL.Query().Get("query")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	query = strings.TrimSpace(query)
	limitStr = strings.TrimSpace(limitStr)
	offsetStr = strings.TrimSpace(offsetStr)

	var limit int
	if limitStr == "" {
		limit = 0
	} else {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			return bookmarks.ListQuery{}, err
		}
	}

	var offset int
	if offsetStr == "" {
		offset = 0
	} else {
		var err error
		offset, err = strconv.Atoi(offsetStr)
		if err != nil {
			return bookmarks.ListQuery{}, err
		}
	}

	if limit < 0 || offset < 0 {
		return bookmarks.ListQuery{}, errors.New("limit and offset must be non-negative")
	}

	return bookmarks.ListQuery{
		Query:  query,
		Limit:  limit,
		Offset: offset,
	}, nil
}
