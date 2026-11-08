// Package server содержит HTTP-хендлеры: JSON API и веб-страницу.
package server

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"searchtags/internal/normalize"
	"searchtags/internal/store"
)

const maxQueryRunes = 256

//go:embed templates/index.html
var indexHTML string

type Server struct {
	store   store.TagStore
	log     *slog.Logger
	timeout time.Duration
	page    *template.Template
}

func New(s store.TagStore, log *slog.Logger, timeout time.Duration) *Server {
	return &Server{
		store:   s,
		log:     log,
		timeout: timeout,
		// html/template сам экранирует всё, что подставляется в страницу.
		page: template.Must(template.New("index").Parse(indexHTML)),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tags", s.handleAPITags)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	return s.logRequests(mux)
}

// validateQuery нормализует запрос и возвращает текст ошибки, если запрос непригоден.
func validateQuery(raw string) (q string, errMsg string) {
	q = normalize.Normalize(raw)
	switch {
	case q == "":
		return "", "Введите запрос."
	case utf8.RuneCountInString(q) > maxQueryRunes:
		return "", "Запрос слишком длинный."
	}
	return q, ""
}

// fetch ходит в хранилище с таймаутом и переводит ошибку в HTTP-статус.
func (s *Server) fetch(r *http.Request, q string) ([]store.Tag, int) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	tags, err := s.store.Tags(ctx, q)
	if err != nil {
		s.log.Error("load tags failed", "query", q, "err", err)
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, http.StatusGatewayTimeout
		}
		return nil, http.StatusInternalServerError
	}
	if tags == nil {
		tags = []store.Tag{} // в JSON должен быть [], а не null
	}
	return tags, http.StatusOK
}

type tagsResponse struct {
	Query string      `json:"query"`
	Tags  []store.Tag `json:"tags"`
}

func (s *Server) handleAPITags(w http.ResponseWriter, r *http.Request) {
	q, msg := validateQuery(r.URL.Query().Get("q"))
	if msg != "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	tags, status := s.fetch(r, q)
	if status != http.StatusOK {
		s.writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
		return
	}
	s.writeJSON(w, http.StatusOK, tagsResponse{Query: q, Tags: tags})
}

type pageData struct {
	Query    string // то, что ввёл пользователь (подставляется в поле, экранируется шаблоном)
	Searched bool
	Tags     []store.Tag
	Error    string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := pageData{}
	status := http.StatusOK

	if r.URL.Query().Has("q") {
		raw := r.URL.Query().Get("q")
		data.Searched = true
		data.Query = raw

		if q, msg := validateQuery(raw); msg != "" {
			data.Error = msg
			status = http.StatusBadRequest
		} else if tags, st := s.fetch(r, q); st != http.StatusOK {
			data.Error = "Сервис временно недоступен, попробуйте позже."
			status = st
		} else {
			data.Tags = tags
		}
	}

	var buf bytes.Buffer
	if err := s.page.Execute(&buf, data); err != nil {
		s.log.Error("render page failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		s.log.Error("encode json failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}
