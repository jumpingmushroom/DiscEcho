package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// GetSettings returns the full key/value map.
func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	all, err := h.Store.GetAllSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, all)
}

// PutSettings accepts a partial map of editable settings, validates
// each known key, and upserts via Store.SetSetting.
//
// Editable keys (and their value types):
//
//	retention.forever  bool
//	retention.days     int >= 1 (when retention.forever is false)
//	library.path       absolute filesystem path
//
// Unknown keys → 422. Cross-key rule: if retention.forever is false,
// retention.days must be present in the PATCH (or already stored)
// and >= 1.
func (h *Handlers) PutSettings(w http.ResponseWriter, r *http.Request) {
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}

	encoded, errs := validateSettingsPatch(patch)

	// Cross-key retention check: only when forever is explicitly false.
	if forever, present := patch["retention.forever"]; present {
		if b, ok := forever.(bool); ok && !b {
			daysVal, daysPresent := patch["retention.days"]
			if !daysPresent {
				existing, _ := h.Store.GetSetting(r.Context(), "retention.days")
				n, _ := strconv.Atoi(existing)
				if n < 1 {
					errs = append(errs, ValidationError{
						Field: "retention.days",
						Msg:   "must be >= 1 when forever is false",
					})
				}
			} else if d, ok := daysVal.(float64); !ok || int(d) < 1 {
				errs = append(errs, ValidationError{
					Field: "retention.days",
					Msg:   "must be >= 1 when forever is false",
				})
			}
		}
	}

	if len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}
	for k, v := range encoded {
		if err := h.Store.SetSetting(r.Context(), k, v); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if h.Broadcaster != nil {
		h.Broadcaster.Publish(state.Event{
			Name:    "settings.changed",
			Payload: map[string]any{"keys": settingsKeys(encoded)},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// allowedSettings maps each editable key to a validator that returns
// the encoded string value or an error.
var allowedSettings = map[string]func(any) (string, error){
	"retention.forever": func(v any) (string, error) {
		b, ok := v.(bool)
		if !ok {
			return "", fmt.Errorf("must be boolean")
		}
		return strconv.FormatBool(b), nil
	},
	"retention.days": func(v any) (string, error) {
		f, ok := v.(float64)
		if !ok {
			return "", fmt.Errorf("must be integer")
		}
		n := int(f)
		if n < 0 {
			return "", fmt.Errorf("must be >= 0")
		}
		return strconv.Itoa(n), nil
	},
	"library.path": func(v any) (string, error) {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("must be string")
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return "", fmt.Errorf("must not be empty")
		}
		if !filepath.IsAbs(s) {
			return "", fmt.Errorf("must be an absolute path")
		}
		return s, nil
	},
}

func validateSettingsPatch(patch map[string]any) (encoded map[string]string, errs []ValidationError) {
	encoded = make(map[string]string, len(patch))
	for k, v := range patch {
		validator, ok := allowedSettings[k]
		if !ok {
			errs = append(errs, ValidationError{Field: k, Msg: "unknown setting key"})
			continue
		}
		s, err := validator(v)
		if err != nil {
			errs = append(errs, ValidationError{Field: k, Msg: err.Error()})
			continue
		}
		encoded[k] = s
	}
	return encoded, errs
}

func settingsKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
