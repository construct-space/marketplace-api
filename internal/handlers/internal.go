package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"construct/marketplace/internal/database"
	"construct/marketplace/internal/models"
)

// Internal endpoints — gateway-secret-gated. Called by other services
// (developer-api on approve/unpublish), never by the public.

// PromoteSpace — POST /internal/spaces
//
// developer-api calls this when an admin approves a publish. Body matches
// the Space row shape; we upsert by name (re-promotion of an existing row
// updates manifest + version, preserves trending-managed counters).
func PromoteSpace(w http.ResponseWriter, r *http.Request) {
	var body models.Space
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	if body.Name == "" {
		writeJSON(w, 400, map[string]any{"error": "id (slug) required"})
		return
	}
	// Visibility defaults to public when not supplied; the schema enforces
	// that visibility='org' rows must carry an owner_org_id, so we re-check
	// here to fail fast with a useful error rather than a 500 from the DB.
	if body.Visibility == "" {
		body.Visibility = "public"
	}
	if body.Visibility != "public" && body.Visibility != "org" {
		writeJSON(w, 400, map[string]any{"error": "visibility must be 'public' or 'org'"})
		return
	}
	if body.Visibility == "org" && (body.OwnerOrgID == nil || *body.OwnerOrgID == "") {
		writeJSON(w, 400, map[string]any{"error": "visibility='org' requires owner_org_id"})
		return
	}
	body.PromotedAt = time.Now().UTC()
	body.DemotedAt = nil

	var existing models.Space
	tx := database.DB.Where("name = ?", body.Name).First(&existing)
	if tx.Error == nil {
		// Preserve trending-managed counters; everything else is replaced
		// with the latest publisher data.
		body.Installs7d = existing.Installs7d
		body.Installs30d = existing.Installs30d
		body.Downloads = existing.Downloads
		if err := database.DB.Save(&body).Error; err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
	} else {
		if err := database.DB.Create(&body).Error; err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "promoted": body.Name})
}

// DemoteSpace — DELETE /internal/spaces/{name}
//
// Soft-delete: sets demoted_at, removes the row from the public list, but
// keeps it around for analytics + re-promotion if the publisher fixes
// whatever got it pulled.
func DemoteSpace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	now := time.Now().UTC()
	res := database.DB.Model(&models.Space{}).
		Where("name = ?", name).
		Update("demoted_at", now)
	if res.Error != nil {
		writeJSON(w, 500, map[string]any{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "demoted": name})
}

// RecordInstall — POST /internal/install-events
//
// Desktop calls this through the gateway after a successful install.
// Insert-only; nightly rollup updates the spaces.installs_7d / 30d
// counters that drive trending.
func RecordInstall(w http.ResponseWriter, r *http.Request) {
	var ev models.InstallEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	if ev.SpaceName == "" {
		writeJSON(w, 400, map[string]any{"error": "space_id required"})
		return
	}
	ev.CreatedAt = time.Now().UTC()
	if err := database.DB.Create(&ev).Error; err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(204)
}
