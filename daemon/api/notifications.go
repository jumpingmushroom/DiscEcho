package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Apprise is the subset of *tools.Apprise the API handler uses. Kept
// as an interface so tests can plug in a fake.
type Apprise interface {
	DryRun(ctx context.Context, url string) error
	Send(ctx context.Context, urls []string, title, body string) error
}

var validTriggers = map[string]bool{"done": true, "failed": true, "warn": true}

// ListNotifications returns every notification row.
func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request) {
	ns, err := h.Store.ListNotifications(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ns)
}

// CreateNotification validates and inserts a notification.
// 201 + the inserted row on success; 422 with field errors on invalid input.
func (h *Handlers) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var n state.Notification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	if errs := validateNotification(r.Context(), &n, h.Apprise); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	if err := h.Store.CreateNotification(r.Context(), &n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "notification.changed",
			Payload: map[string]any{"notification": n},
		})
	}
	writeJSON(w, http.StatusCreated, n)
}

// UpdateNotification validates and replaces a notification. ID comes
// from the URL; the body's id field is ignored.
// 200 + the updated row on success; 404 / 422 on error.
func (h *Handlers) UpdateNotification(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var n state.Notification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	n.ID = id
	if errs := validateNotification(r.Context(), &n, h.Apprise); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	if err := h.Store.UpdateNotification(r.Context(), &n); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fresh, err := h.Store.GetNotification(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "notification.changed",
			Payload: map[string]any{"notification": *fresh},
		})
	}
	writeJSON(w, http.StatusOK, fresh)
}

// DeleteNotification removes a notification. 204 on success; 404 if missing.
func (h *Handlers) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteNotification(r.Context(), id); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, "notification not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "notification.changed",
			Payload: map[string]any{"notification_id": id, "deleted": true},
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateNotification checks name length, URL via Apprise dry-run,
// and triggers against the allowlist.
func validateNotification(ctx context.Context, n *state.Notification, ap Apprise) []ValidationError {
	var errs []ValidationError

	name := strings.TrimSpace(n.Name)
	if name == "" {
		errs = append(errs, ValidationError{Field: "name", Msg: "required"})
	} else if len(name) > 64 {
		errs = append(errs, ValidationError{Field: "name", Msg: "max 64 chars"})
	}

	url := strings.TrimSpace(n.URL)
	if url == "" {
		errs = append(errs, ValidationError{Field: "url", Msg: "required"})
	} else if ap != nil {
		if err := ap.DryRun(ctx, url); err != nil {
			errs = append(errs, ValidationError{Field: "url", Msg: err.Error()})
		}
	}

	for _, t := range strings.Split(n.Triggers, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !validTriggers[t] {
			errs = append(errs, ValidationError{Field: "triggers", Msg: "unknown trigger: " + t})
			break // one error per field is sufficient
		}
	}

	return errs
}
