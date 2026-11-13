package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/oc/oc-broker/internal/store"
)

type API struct {
	store  *store.Store
	logger *slog.Logger
}

func New(s *store.Store, logger *slog.Logger) http.Handler {
	a := &API{store: s, logger: logger}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, a.logRequests)
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/health/ready", a.ready)
	r.Route("/v1", func(r chi.Router) {
		r.Post("/claimants", a.createClaimant)
		r.Get("/claimants", a.listClaimants)
		r.Get("/claimants/{claimantID}", a.getClaimant)
		r.Get("/claimants/{claimantID}/claims", a.listClaims)
		r.Post("/claims", a.createClaim)
		r.Get("/claims/{claimID}", a.getClaim)
	})
	return r
}

type claimantListQuery struct {
	Search string
	Limit  int
	Offset int
}

func parseClaimantListQuery(values url.Values) (claimantListQuery, error) {
	query := claimantListQuery{Search: strings.TrimSpace(values.Get("search")), Limit: 20}
	if utf8.RuneCountInString(query.Search) > 100 {
		return claimantListQuery{}, errors.New("search must contain at most 100 characters")
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return claimantListQuery{}, errors.New("limit must be an integer between 1 and 100")
		}
		query.Limit = limit
	}
	if raw := values.Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return claimantListQuery{}, errors.New("offset must be a non-negative integer")
		}
		query.Offset = offset
	}
	return query, nil
}

func (a *API) listClaimants(w http.ResponseWriter, r *http.Request) {
	query, err := parseClaimantListQuery(r.URL.Query())
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := a.store.ListClaimants(r.Context(), query.Search, query.Limit, query.Offset)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  page.Items,
		"total":  page.Total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
}

type createClaimantRequest struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
}

func (a *API) createClaimant(w http.ResponseWriter, r *http.Request) {
	var req createClaimantRequest
	if err := decodeJSON(w, r, &req); err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(strings.TrimSpace(req.FullName)) < 2 {
		problem(w, http.StatusUnprocessableEntity, "fullName must contain at least 2 characters")
		return
	}
	parsedEmail, err := mail.ParseAddress(req.Email)
	if err != nil || parsedEmail.Address != strings.TrimSpace(req.Email) {
		problem(w, http.StatusUnprocessableEntity, "email must be valid")
		return
	}
	c, err := a.store.CreateClaimant(r.Context(), req.FullName, req.Email)
	if errors.Is(err, store.ErrEmailExists) {
		problem(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/claimants/"+c.ID.String())
	writeJSON(w, http.StatusCreated, c)
}

func (a *API) getClaimant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "claimantID")
	if !ok {
		return
	}
	c, err := a.store.GetClaimant(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		problem(w, http.StatusNotFound, "claimant not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type createClaimRequest struct {
	ClaimantID     string `json:"claimantId"`
	PolicyNumber   string `json:"policyNumber"`
	IncidentDate   string `json:"incidentDate"`
	AmountCents    int64  `json:"amountCents"`
	Currency       string `json:"currency"`
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func (a *API) createClaim(w http.ResponseWriter, r *http.Request) {
	var req createClaimRequest
	if err := decodeJSON(w, r, &req); err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	claimantID, err := uuid.Parse(req.ClaimantID)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "claimantId must be a UUID")
		return
	}
	incidentDate, err := time.Parse("2006-01-02", req.IncidentDate)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "incidentDate must use YYYY-MM-DD")
		return
	}
	if incidentDate.After(time.Now().UTC().Truncate(24 * time.Hour)) {
		problem(w, http.StatusUnprocessableEntity, "incidentDate cannot be in the future")
		return
	}
	if req.AmountCents <= 0 {
		problem(w, http.StatusUnprocessableEntity, "amountCents must be positive")
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency != "GBP" && currency != "EUR" && currency != "USD" {
		problem(w, http.StatusUnprocessableEntity, "currency must be GBP, EUR, or USD")
		return
	}
	if len(strings.TrimSpace(req.PolicyNumber)) < 3 {
		problem(w, http.StatusUnprocessableEntity, "policyNumber is required")
		return
	}
	if len(strings.TrimSpace(req.Description)) < 10 {
		problem(w, http.StatusUnprocessableEntity, "description must contain at least 10 characters")
		return
	}
	if len(strings.TrimSpace(req.IdempotencyKey)) < 8 || len(req.IdempotencyKey) > 100 {
		problem(w, http.StatusUnprocessableEntity, "idempotencyKey must contain 8 to 100 characters")
		return
	}

	c, created, err := a.store.CreateClaim(r.Context(), store.NewClaim{ClaimantID: claimantID, PolicyNumber: strings.TrimSpace(req.PolicyNumber), IncidentDate: incidentDate, AmountCents: req.AmountCents, Currency: currency, Description: strings.TrimSpace(req.Description), IdempotencyKey: strings.TrimSpace(req.IdempotencyKey)})
	if errors.Is(err, store.ErrNotFound) {
		problem(w, http.StatusUnprocessableEntity, "claimant not found")
		return
	}
	if errors.Is(err, store.ErrInactiveClaimant) {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/claims/"+c.ID.String())
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, c)
}

func (a *API) getClaim(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "claimID")
	if !ok {
		return
	}
	c, err := a.store.GetClaim(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		problem(w, http.StatusNotFound, "claim not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (a *API) listClaims(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "claimantID")
	if !ok {
		return
	}
	if _, err := a.store.GetClaimant(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		problem(w, http.StatusNotFound, "claimant not found")
		return
	} else if err != nil {
		a.internalError(w, r, err)
		return
	}
	claims, err := a.store.ListClaims(r.Context(), id)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": claims, "count": len(claims)})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.store.Pool.Ping(ctx); err != nil {
		problem(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) internalError(w http.ResponseWriter, r *http.Request, err error) {
	a.logger.Error("request failed", "error", err, "request_id", middleware.GetReqID(r.Context()))
	problem(w, http.StatusInternalServerError, "internal server error")
}

func (a *API) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		a.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(r.Context()))
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		problem(w, http.StatusBadRequest, name+" must be a UUID")
		return uuid.Nil, false
	}
	return id, true
}

func problem(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"status": status, "title": http.StatusText(status), "detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
