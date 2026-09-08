package main

import (
	"log"
	"net/http"
	"time"

	"construct/marketplace/internal/config"
	"construct/marketplace/internal/database"
	"construct/marketplace/internal/handlers"
	"construct/marketplace/internal/middleware"
)

func main() {
	cfg := config.Load()
	database.Init(cfg)

	mux := http.NewServeMux()

	// Health (public)
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /api/health", handlers.Health)

	// Public read-only marketplace surface
	mux.HandleFunc("GET /api/marketplace/spaces", handlers.ListSpaces)
	mux.HandleFunc("GET /api/marketplace/spaces/{name}", handlers.GetSpace)
	mux.HandleFunc("GET /api/marketplace/categories", handlers.ListCategories)
	mux.HandleFunc("GET /api/marketplace/collections", handlers.ListCollections)
	mux.HandleFunc("GET /api/marketplace/collections/{slug}", handlers.GetCollection)

	// Internal — gateway-secret-gated. developer-api + desktop install pings.
	mux.Handle("POST /internal/spaces", middleware.AdminAuth(http.HandlerFunc(handlers.PromoteSpace)))
	mux.Handle("DELETE /internal/spaces/{name}", middleware.AdminAuth(http.HandlerFunc(handlers.DemoteSpace)))
	mux.Handle("POST /internal/install-events", middleware.AdminAuth(http.HandlerFunc(handlers.RecordInstall)))

	// Admin — secret-gated, used by oracle-web staff curation UI.
	// Spaces (catalog rows) — list, patch (category/tags), editorial.
	mux.Handle("GET /api/admin/spaces", middleware.AdminAuth(http.HandlerFunc(handlers.AdminListSpaces)))
	mux.Handle("PATCH /api/admin/spaces/{name}", middleware.AdminAuth(http.HandlerFunc(handlers.AdminPatchSpace)))
	mux.Handle("GET /api/admin/spaces/{name}/editorial", middleware.AdminAuth(http.HandlerFunc(handlers.AdminGetEditorial)))
	mux.Handle("PUT /api/admin/spaces/{name}/editorial", middleware.AdminAuth(http.HandlerFunc(handlers.AdminSetEditorial)))

	// Categories (lookup)
	mux.Handle("GET /api/admin/categories", middleware.AdminAuth(http.HandlerFunc(handlers.AdminListCategoriesAll)))
	mux.Handle("POST /api/admin/categories", middleware.AdminAuth(http.HandlerFunc(handlers.AdminCreateCategory)))
	mux.Handle("PUT /api/admin/categories/{slug}", middleware.AdminAuth(http.HandlerFunc(handlers.AdminPatchCategory)))
	mux.Handle("DELETE /api/admin/categories/{slug}", middleware.AdminAuth(http.HandlerFunc(handlers.AdminDeleteCategory)))

	// Collections (curation)
	mux.Handle("GET /api/admin/collections", middleware.AdminAuth(http.HandlerFunc(handlers.AdminListCollectionsAll)))
	mux.Handle("GET /api/admin/collections/{id}", middleware.AdminAuth(http.HandlerFunc(handlers.AdminGetCollection)))
	mux.Handle("POST /api/admin/collections", middleware.AdminAuth(http.HandlerFunc(handlers.AdminCreateCollection)))
	mux.Handle("PUT /api/admin/collections/{id}", middleware.AdminAuth(http.HandlerFunc(handlers.AdminPatchCollection)))
	mux.Handle("DELETE /api/admin/collections/{id}", middleware.AdminAuth(http.HandlerFunc(handlers.AdminDeleteCollection)))
	mux.Handle("POST /api/admin/collections/{id}/spaces", middleware.AdminAuth(http.HandlerFunc(handlers.AdminAddSpaceToCollection)))
	mux.Handle("DELETE /api/admin/collections/{id}/spaces/{name}", middleware.AdminAuth(http.HandlerFunc(handlers.AdminRemoveSpaceFromCollection)))
	mux.Handle("PUT /api/admin/collections/{id}/order", middleware.AdminAuth(http.HandlerFunc(handlers.AdminReorderCollection)))

	// Wrap with logger + CORS. CORS is intentionally permissive on the
	// public namespace because the desktop app, future website, and
	// third-party clients all read it; oracle-web origins are listed in
	// ALLOWED_ORIGINS for the admin lane.
	handler := middleware.Logger(middleware.CORS(cfg)(mux))

	log.Printf("marketplace-api listening on :%s", cfg.Port)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
