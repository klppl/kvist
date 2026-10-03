// Package api exposes the sync protocol over HTTP (§3). Routes live under
// /api/v1/; everything except /api/v1/info needs a per-site bearer token.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/klppl/kvist/internal/auth"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/syncer"
)

// maxManifestBytes bounds the JSON body of POST /syncs (20 000 entries need
// roughly 6 MB).
const maxManifestBytes = 64 << 20

// maxWait bounds the long-poll of GET /builds/{id}.
const maxWait = 60 * time.Second

// Server is the HTTP API.
type Server struct {
	svc           *syncer.Service
	tokens        *auth.File
	serverVersion string
	log           *slog.Logger
	mux           *http.ServeMux
}

// New returns the API handler.
func New(svc *syncer.Service, tokens *auth.File, serverVersion string, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{svc: svc, tokens: tokens, serverVersion: serverVersion, log: log, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/v1/info", s.info)
	s.mux.HandleFunc("GET /api/v1/sites/{site}", s.authed(s.siteInfo))
	s.mux.HandleFunc("POST /api/v1/sites/{site}/syncs", s.authed(s.startSync))
	s.mux.HandleFunc("PUT /api/v1/sites/{site}/syncs/{id}/blobs/{hash}", s.authed(s.putBlob))
	s.mux.HandleFunc("POST /api/v1/sites/{site}/syncs/{id}/commit", s.authed(s.commit))
	s.mux.HandleFunc("GET /api/v1/sites/{site}/builds/{id}", s.authed(s.buildStatus))
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, &syncer.Error{Status: http.StatusNotFound, Wire: protocol.Error{Code: protocol.ErrNotFound, Message: "not found"}})
	})
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(protocol.HeaderProtocol, strconv.Itoa(protocol.Version))
	w.Header().Set(protocol.HeaderServerVersion, s.serverVersion)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if v := r.Header.Get(protocol.HeaderProtocol); v != "" || r.URL.Path != "/api/v1/info" {
		n, err := strconv.Atoi(v)
		if err != nil || n < protocol.MinVersion || n > protocol.MaxVersion {
			s.writeError(w, &syncer.Error{Status: http.StatusBadRequest, Wire: protocol.Error{
				Code:    protocol.ErrProtocolUnsupported,
				Message: "unsupported or missing " + protocol.HeaderProtocol + " header; update the plugin or the server",
			}})
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			s.writeError(w, unauthorized())
			return
		}
		if _, ok := s.tokens.Authenticate(strings.TrimSpace(secret), r.PathValue("site"), auth.ScopePush); !ok {
			s.writeError(w, unauthorized())
			return
		}
		h(w, r)
	}
}

func unauthorized() *syncer.Error {
	return &syncer.Error{Status: http.StatusUnauthorized, Wire: protocol.Error{Code: protocol.ErrUnauthorized, Message: "invalid token"}}
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, protocol.Info{
		Protocol: protocol.VersionRange{Min: protocol.MinVersion, Max: protocol.MaxVersion},
		Server:   s.serverVersion,
	})
}

func (s *Server) siteInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.svc.SiteInfo(r.PathValue("site"))
	s.respond(w, info, err)
}

func (s *Server) startSync(w http.ResponseWriter, r *http.Request) {
	var m protocol.Manifest
	if !s.decode(w, r, maxManifestBytes, &m) {
		return
	}
	resp, err := s.svc.StartSync(r.PathValue("site"), m)
	s.respond(w, resp, err)
}

func (s *Server) putBlob(w http.ResponseWriter, r *http.Request) {
	err := s.svc.PutBlob(r.PathValue("site"), r.PathValue("id"), r.PathValue("hash"), r.Body)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) commit(w http.ResponseWriter, r *http.Request) {
	var req protocol.CommitRequest
	if r.ContentLength != 0 {
		if !s.decode(w, r, 1<<16, &req) {
			return
		}
	}
	resp, err := s.svc.Commit(r.PathValue("site"), r.PathValue("id"), req)
	s.respond(w, resp, err)
}

func (s *Server) buildStatus(w http.ResponseWriter, r *http.Request) {
	var wait time.Duration
	if v := r.URL.Query().Get("wait"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			s.writeError(w, badRequest("invalid wait duration"))
			return
		}
		wait = min(d, maxWait)
	}
	st, err := s.svc.BuildStatus(r.Context(), r.PathValue("site"), r.PathValue("id"), wait)
	s.respond(w, st, err)
}

func badRequest(msg string) *syncer.Error {
	return &syncer.Error{Status: http.StatusBadRequest, Wire: protocol.Error{Code: protocol.ErrBadRequest, Message: msg}}
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			s.writeError(w, &syncer.Error{Status: http.StatusRequestEntityTooLarge, Wire: protocol.Error{Code: protocol.ErrTooLarge, Message: "request body too large"}})
		} else {
			s.writeError(w, badRequest("invalid JSON body"))
		}
		return false
	}
	return true
}

func (s *Server) respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	var se *syncer.Error
	if !errors.As(err, &se) {
		s.log.Error("internal error", "err", err)
		se = &syncer.Error{Status: http.StatusInternalServerError, Wire: protocol.Error{Code: protocol.ErrInternal, Message: "internal error"}}
	}
	writeJSON(w, se.Status, protocol.ErrorBody{Error: &se.Wire})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
