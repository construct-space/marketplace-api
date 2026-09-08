package handlers

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"construct/marketplace/internal/database"
	"construct/marketplace/internal/models"

	"github.com/lib/pq"
)

// Admin endpoints — gateway-secret-gated. Used by oracle-web's marketplace
// curation surface to manage categories, tags, collections, and editorial
// overrides. All routes mutate; the public read API stays untouched.

// ---------- Spaces (admin view) ----------

// AdminListSpaces — GET /api/admin/spaces?q=&category=&page=&pageSize=&includeDemoted=
//
// Like the public list but unfiltered (no demoted_at filter unless caller
// asks otherwise) and with editorial overrides joined in so the UI can
// show "publisher value vs editorial override" side-by-side.
func AdminListSpaces(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	page := atoiOr(r.URL.Query().Get("page"), 1)
	pageSize := clamp(atoiOr(r.URL.Query().Get("pageSize"), 50), 1, 200)
	includeDemoted := r.URL.Query().Get("includeDemoted") == "true"

	tx := database.DB.Model(&models.Space{})
	if !includeDemoted {
		tx = tx.Where("demoted_at IS NULL")
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("LOWER(name) LIKE ? OR LOWER(title) LIKE ?", like, like)
	}
	if category != "" {
		tx = tx.Where("category_slug = ?", category)
	}

	var total int64
	tx.Count(&total)

	var rows []models.Space
	tx.Order("promoted_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&rows)

	writeJSON(w, 200, map[string]any{
		"spaces":   rows,
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
	})
}

// AdminPatchSpace — PATCH /api/admin/spaces/{name}
//
// Updates the staff-managed columns: category_slug, tags. Publisher fields
// (title/description/icon/manifest) are sourced from developer-api and
// not editable here — staff overrides go through the editorial endpoint.
func AdminPatchSpace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Category *string  `json:"category"`
		Tags     []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}

	updates := map[string]any{}
	if body.Category != nil {
		// Empty string clears the category (back to uncategorized).
		if *body.Category == "" {
			updates["category_slug"] = nil
		} else {
			updates["category_slug"] = *body.Category
		}
	}
	if body.Tags != nil {
		// Postgres text[] via lib/pq
		updates["tags"] = pq.StringArray(body.Tags)
	}
	if len(updates) == 0 {
		writeJSON(w, 400, map[string]any{"error": "no editable fields in body"})
		return
	}
	res := database.DB.Model(&models.Space{}).Where("name = ?", name).Updates(updates)
	if res.Error != nil {
		writeJSON(w, 500, map[string]any{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		writeJSON(w, 404, map[string]any{"error": "space not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// AdminSetEditorial — PUT /api/admin/spaces/{name}/editorial
//
// Upserts the per-space editorial overrides row.
func AdminSetEditorial(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body models.EditorialOverride
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	body.SpaceName = name
	body.UpdatedAt = time.Now().UTC()

	// Upsert by primary key
	if err := database.DB.Save(&body).Error; err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, body)
}

// AdminGetEditorial — GET /api/admin/spaces/{name}/editorial
func AdminGetEditorial(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var ed models.EditorialOverride
	if err := database.DB.Where("space_name = ?", name).First(&ed).Error; err != nil {
		writeJSON(w, 404, map[string]any{"error": "no editorial overrides"})
		return
	}
	writeJSON(w, 200, ed)
}

// ---------- Categories (admin CRUD) ----------

// AdminListCategoriesAll — GET /api/admin/categories
// Includes hidden categories (visible=false), unlike the public route.
func AdminListCategoriesAll(w http.ResponseWriter, r *http.Request) {
	var cats []models.Category
	database.DB.Order("position ASC").Find(&cats)
	writeJSON(w, 200, map[string]any{"categories": cats})
}

// AdminCreateCategory — POST /api/admin/categories
func AdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	var body models.Category
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	if body.Slug == "" || body.Title == "" {
		writeJSON(w, 400, map[string]any{"error": "slug and title required"})
		return
	}
	if err := database.DB.Create(&body).Error; err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 201, body)
}

// AdminPatchCategory — PUT /api/admin/categories/{slug}
func AdminPatchCategory(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var body models.Category
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	body.Slug = slug // path wins
	res := database.DB.Save(&body)
	if res.Error != nil {
		writeJSON(w, 500, map[string]any{"error": res.Error.Error()})
		return
	}
	writeJSON(w, 200, body)
}

// AdminDeleteCategory — DELETE /api/admin/categories/{slug}
//
// Spaces with this category get their category_slug cleared (FK is
// ON DELETE SET NULL).
func AdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	res := database.DB.Where("slug = ?", slug).Delete(&models.Category{})
	if res.Error != nil {
		writeJSON(w, 500, map[string]any{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- Collections (admin CRUD) ----------

// AdminListCollectionsAll — GET /api/admin/collections
// Includes inactive collections, unlike the public route.
func AdminListCollectionsAll(w http.ResponseWriter, r *http.Request) {
	var cols []models.Collection
	database.DB.Order("priority ASC").Find(&cols)
	writeJSON(w, 200, map[string]any{"collections": cols})
}

// AdminCreateCollection — POST /api/admin/collections
func AdminCreateCollection(w http.ResponseWriter, r *http.Request) {
	var body models.Collection
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	if body.Slug == "" || body.Title == "" || body.Kind == "" {
		writeJSON(w, 400, map[string]any{"error": "slug, title, kind required"})
		return
	}
	if body.ID == "" {
		body.ID = newUUIDv4()
	}
	if err := database.DB.Create(&body).Error; err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 201, body)
}

// AdminPatchCollection — PUT /api/admin/collections/{id}
func AdminPatchCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body models.Collection
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	body.ID = id
	res := database.DB.Save(&body)
	if res.Error != nil {
		writeJSON(w, 500, map[string]any{"error": res.Error.Error()})
		return
	}
	writeJSON(w, 200, body)
}

// AdminDeleteCollection — DELETE /api/admin/collections/{id}
func AdminDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	database.DB.Where("collection_id = ?", id).Delete(&models.CollectionSpace{})
	res := database.DB.Where("id = ?", id).Delete(&models.Collection{})
	if res.Error != nil {
		writeJSON(w, 500, map[string]any{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// AdminGetCollection — GET /api/admin/collections/{id}
//
// Returns the collection plus its spaces in stored order (matches the
// public GetCollection but uses the admin filter — sees demoted spaces).
func AdminGetCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var col models.Collection
	if err := database.DB.Where("id = ?", id).First(&col).Error; err != nil {
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	var spaces []models.Space
	database.DB.
		Joins("JOIN collection_spaces cs ON cs.space_name = spaces.name").
		Where("cs.collection_id = ?", id).
		Order("cs.position ASC").
		Find(&spaces)
	writeJSON(w, 200, map[string]any{"collection": col, "spaces": spaces})
}

// AdminAddSpaceToCollection — POST /api/admin/collections/{id}/spaces
// Body: { "space_name": "...", "position": 5 }
func AdminAddSpaceToCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		SpaceName string `json:"space_name"`
		Position  int    `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SpaceName == "" {
		writeJSON(w, 400, map[string]any{"error": "space_name required"})
		return
	}
	if body.Position == 0 {
		// Append: position = max(existing) + 1
		var maxPos int
		database.DB.Model(&models.CollectionSpace{}).
			Where("collection_id = ?", id).
			Select("COALESCE(MAX(position), 0)").Scan(&maxPos)
		body.Position = maxPos + 1
	}
	row := models.CollectionSpace{
		CollectionID: id,
		SpaceName:    body.SpaceName,
		Position:     body.Position,
		AddedAt:      time.Now().UTC(),
	}
	if err := database.DB.Create(&row).Error; err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 201, row)
}

// AdminRemoveSpaceFromCollection — DELETE /api/admin/collections/{id}/spaces/{name}
func AdminRemoveSpaceFromCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")
	res := database.DB.Where("collection_id = ? AND space_name = ?", id, name).Delete(&models.CollectionSpace{})
	if res.Error != nil {
		writeJSON(w, 500, map[string]any{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		writeJSON(w, 404, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// AdminReorderCollection — PUT /api/admin/collections/{id}/order
// Body: { "order": ["space-a", "space-b", ...] }
//
// Atomically rewrites positions for the collection. Drag-drop save in
// the staff UI calls this after a reorder.
func AdminReorderCollection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid body"})
		return
	}
	tx := database.DB.Begin()
	for i, name := range body.Order {
		if err := tx.Model(&models.CollectionSpace{}).
			Where("collection_id = ? AND space_name = ?", id, name).
			Update("position", i+1).Error; err != nil {
			tx.Rollback()
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- helpers ----------

// newUUIDv4 generates a v4 UUID. Avoids adding google/uuid as a hard
// dep for one-off ID minting; format matches the uuid type in Postgres.
func newUUIDv4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	for i, j := 0, 0; i < 16; i++ {
		if j == 8 || j == 13 || j == 18 || j == 23 {
			out[j] = '-'
			j++
		}
		out[j] = hex[b[i]>>4]
		out[j+1] = hex[b[i]&0x0f]
		j += 2
	}
	return string(out)
}
