package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"construct/marketplace/internal/database"
	"construct/marketplace/internal/gwauth"
	"construct/marketplace/internal/models"

	"gorm.io/gorm"
)

// callerOrgID returns the gateway-asserted org id for the caller, or "" if
// the request didn't pass through the gateway with an org-scoped session.
// Personal-scope sessions and anonymous calls both yield "" here — both
// see only public spaces.
func callerOrgID(r *http.Request) string {
	id := gwauth.Gateway(r)
	if id == nil || id.Scope != "org" {
		return ""
	}
	return id.OrgID
}

// applyVisibilityScope narrows a query so the caller can only see spaces
// they're allowed to. Public rows always pass; visibility='org' rows pass
// only when their owner_org_id matches the caller's org. Anonymous and
// personal-scope callers see only public.
func applyVisibilityScope(tx *gorm.DB, orgID string) *gorm.DB {
	if orgID == "" {
		return tx.Where("visibility = ?", "public")
	}
	return tx.Where("visibility = ? OR (visibility = ? AND owner_org_id = ?)",
		"public", "org", orgID)
}

// Public, read-only marketplace endpoints. No auth required, GET only.
// CORS is wide open at the middleware layer — these routes are intended
// for public consumption (desktop app, future website, third-party).

// ListSpaces — GET /api/marketplace/spaces?q=&category=&tag=&page=&pageSize=
func ListSpaces(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	page := atoiOr(r.URL.Query().Get("page"), 1)
	pageSize := clamp(atoiOr(r.URL.Query().Get("pageSize"), 24), 1, 100)
	sortBy := r.URL.Query().Get("sort") // 'recent' | 'installs' | 'trending'

	tx := database.DB.Model(&models.Space{}).Where("demoted_at IS NULL")
	tx = applyVisibilityScope(tx, callerOrgID(r))

	if q != "" {
		// Trigram-friendly ILIKE on name + title
		like := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("LOWER(name) LIKE ? OR LOWER(title) LIKE ?", like, like)
	}
	if category != "" {
		tx = tx.Where("category_slug = ?", category)
	}
	if tag != "" {
		// Postgres array containment — uses idx_spaces_tags_gin
		tx = tx.Where("tags @> ARRAY[?]::text[]", tag)
	}

	switch sortBy {
	case "trending":
		tx = tx.Order("installs7d DESC, promoted_at DESC")
	case "installs":
		tx = tx.Order("installs30d DESC, promoted_at DESC")
	default:
		tx = tx.Order("promoted_at DESC")
	}

	var total int64
	tx.Count(&total)

	var rows []models.Space
	tx.Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows)

	writeJSON(w, 200, map[string]any{
		"spaces":   rows,
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
	})
}

// GetSpace — GET /api/marketplace/spaces/{name}
func GetSpace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var sp models.Space
	tx := database.DB.Where("name = ? AND demoted_at IS NULL", name)
	tx = applyVisibilityScope(tx, callerOrgID(r))
	if err := tx.First(&sp).Error; err != nil {
		// 404 (not found) and "not visible to this caller" intentionally
		// collapse to the same response — don't leak existence of
		// org-private rows to outsiders.
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, 200, sp)
}

// ListCategories — GET /api/marketplace/categories
func ListCategories(w http.ResponseWriter, r *http.Request) {
	var cats []models.Category
	database.DB.Where("visible = ?", true).Order("position ASC").Find(&cats)
	writeJSON(w, 200, map[string]any{"categories": cats})
}

// ListCollections — GET /api/marketplace/collections
//
// Returns currently-active collections (active_from..active_to window
// covers now() — both nullable). Caller follows up with /collections/{slug}
// to expand each one's spaces.
func ListCollections(w http.ResponseWriter, r *http.Request) {
	var cols []models.Collection
	database.DB.
		Where("(active_from IS NULL OR active_from <= now()) AND (active_to IS NULL OR active_to >= now())").
		Order("priority ASC").
		Find(&cols)
	writeJSON(w, 200, map[string]any{"collections": cols})
}

// GetCollection — GET /api/marketplace/collections/{slug}
//
// Manual collections: returns the spaces in stored order.
// Dynamic collections: re-runs the saved query against the live spaces table.
//
// (Dynamic execution is stubbed in this skeleton; a future commit adds the
// query interpreter that translates the saved jsonb into a gorm chain.)
func GetCollection(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var col models.Collection
	if err := database.DB.Where("slug = ?", slug).First(&col).Error; err != nil {
		writeJSON(w, 404, map[string]any{"error": "collection not found"})
		return
	}

	orgID := callerOrgID(r)

	var spaces []models.Space
	if col.Kind == "manual" {
		tx := database.DB.
			Joins("JOIN collection_spaces cs ON cs.space_name = spaces.name").
			Where("cs.collection_id = ? AND spaces.demoted_at IS NULL", col.ID).
			Order("cs.position ASC")
		applyVisibilityScope(tx, orgID).Find(&spaces)
	} else {
		// Dynamic: stub. A real impl interprets col.Query and applies it.
		tx := database.DB.
			Where("demoted_at IS NULL").
			Order("installs7d DESC").
			Limit(24)
		applyVisibilityScope(tx, orgID).Find(&spaces)
	}

	writeJSON(w, 200, map[string]any{
		"collection": col,
		"spaces":     spaces,
	})
}

// helpers

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func atoiOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
