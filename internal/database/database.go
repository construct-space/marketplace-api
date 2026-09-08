package database

import (
	"fmt"
	"log"
	"time"

	"construct/marketplace/internal/config"
	"construct/marketplace/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Init opens the marketplace_db connection and runs gorm AutoMigrate +
// raw SQL for indexes/constraints AutoMigrate can't express (GIN on tags,
// partial indexes, etc.).
func Init(cfg *config.Config) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("connect marketplace_db: %v", err)
	}
	DB = db
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(50)
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := db.AutoMigrate(
		&models.Category{},
		&models.Space{},
		&models.Collection{},
		&models.CollectionSpace{},
		&models.EditorialOverride{},
		&models.InstallEvent{},
	); err != nil {
		log.Fatalf("automigrate: %v", err)
	}

	if err := runRawMigrations(db); err != nil {
		log.Fatalf("raw migrations: %v", err)
	}

	// One-shot: backfill spaces.scopes + spaces.project_aware from the
	// legacy single `scope` column, then drop it. Mapping mirrors the
	// frontend's old normaliser. Idempotent — gated on whether the old
	// column still exists.
	if db.Migrator().HasColumn(&models.Space{}, "scope") {
		mappings := []struct {
			match   string
			scopes  string // Postgres text[] literal, e.g. {app,org}
			project bool
		}{
			{"app", "{app}", false},
			{"org", "{org}", false},
			{"company", "{org}", false},
			{"project", "{app}", true},
			{"both", "{app,org}", true},
		}
		for _, m := range mappings {
			if err := db.Exec(
				"UPDATE spaces SET scopes = ?::text[], project_aware = ? WHERE scope = ? AND (scopes IS NULL OR scopes = '{}')",
				m.scopes, m.project, m.match,
			).Error; err != nil {
				log.Printf("[migrate] backfill scopes for scope=%q: %v", m.match, err)
			}
		}
		if err := db.Migrator().DropColumn(&models.Space{}, "scope"); err != nil {
			log.Printf("[migrate] drop spaces.scope: %v", err)
		}
	}

	log.Printf("connected to postgres: %s@%s:%s", cfg.DBName, cfg.DBHost, cfg.DBPort)
}

// runRawMigrations runs idempotent CREATE INDEX / CREATE EXTENSION for
// things gorm AutoMigrate can't model. Safe to re-run.
func runRawMigrations(db *gorm.DB) error {
	stmts := []string{
		// pg_trgm for fuzzy-search support on space title/description
		`CREATE EXTENSION IF NOT EXISTS pg_trgm`,

		// GIN on tags array — fast "any space tagged with X or Y"
		`CREATE INDEX IF NOT EXISTS idx_spaces_tags_gin ON spaces USING GIN (tags)`,

		// Trigram indexes for ILIKE search on name/title
		`CREATE INDEX IF NOT EXISTS idx_spaces_name_trgm  ON spaces USING GIN (name gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_spaces_title_trgm ON spaces USING GIN (title gin_trgm_ops)`,

		// Partial index: only LIVE rows make it onto the public list
		`CREATE INDEX IF NOT EXISTS idx_spaces_live ON spaces (promoted_at DESC) WHERE demoted_at IS NULL`,

		// Trending compute scans install_events by space + recency
		`CREATE INDEX IF NOT EXISTS idx_install_events_recent ON install_events (space_name, created_at DESC)`,

		// FK from spaces.category_slug → categories.slug; allow nulls
		// (uncategorized spaces) and ON DELETE SET NULL so dropping a
		// category doesn't cascade-delete its spaces.
		`DO $$
		 BEGIN
		   IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints
		                  WHERE constraint_name = 'spaces_category_slug_fkey') THEN
		     ALTER TABLE spaces
		       ADD CONSTRAINT spaces_category_slug_fkey
		       FOREIGN KEY (category_slug) REFERENCES categories(slug)
		       ON DELETE SET NULL;
		   END IF;
		 END $$`,

		// Visibility invariant: 'org' rows must carry an owner_org_id.
		// 'public' rows MAY carry one (it then displays as "by <org>")
		// or not. Caught at the DB layer so a buggy publisher can't
		// accidentally promote an unowned private row.
		`DO $$
		 BEGIN
		   IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints
		                  WHERE constraint_name = 'spaces_visibility_owner_chk') THEN
		     ALTER TABLE spaces
		       ADD CONSTRAINT spaces_visibility_owner_chk
		       CHECK (visibility IN ('public','org')
		              AND (visibility = 'public' OR owner_org_id IS NOT NULL));
		   END IF;
		 END $$`,

		// Composite index for org-private listings — every authenticated
		// list query filters on (visibility, owner_org_id) for the
		// caller's org branch.
		`CREATE INDEX IF NOT EXISTS idx_spaces_visibility_owner
		   ON spaces (visibility, owner_org_id)
		   WHERE demoted_at IS NULL`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return fmt.Errorf("raw stmt: %w", err)
		}
	}
	return nil
}
