package models

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// Space is the public, live catalog row. Created by developer-api when a
// publish is approved (POST /internal/spaces) and removed when unpublished.
//
// This is NOT the publishing-side row (drafts, in-review submissions live
// in developer_db). Marketplace only stores the parts of a space the
// catalog needs: identity, descriptive metadata, the latest published
// tarball, and curation hooks (category, tags, editorial overrides).
//
// Identity: the slug (Name) is the primary key. Same convention as
// developer-api which keys spaces by their human-readable slug; matching
// that here keeps cross-service refs trivial (no uuid mapping table).
type Space struct {
	Name        string `gorm:"primaryKey;size:100"      json:"id"`             // slug, e.g. "kanban-board"
	Title       string `gorm:"not null"                 json:"name"`           // display name, "Kanban Board"
	Description string `                                  json:"description"`
	Icon        string `                                  json:"icon"`          // URL or lucide name
	Version     string `gorm:"not null"                 json:"version"`        // semver of the latest live tarball
	HostAPIVer  string `                                  json:"host_api_version"`
	Manifest    json.RawMessage `gorm:"type:jsonb"      json:"manifest,omitempty"`
	TarballURL  string `                                  json:"tarball_url"`   // R2 URL
	// Scopes lists the surfaces the space publishes itself on. Allowed
	// values: "app" | "org". GIN-indexed for catalog filters that ask
	// "give me everything visible from the org sidebar" etc.
	Scopes      pq.StringArray `gorm:"type:text[];index:idx_spaces_scopes,using:gin" json:"scopes"`
	// ProjectAware is the orthogonal flag for spaces that participate in
	// project context (project-bound graph models, project page surfaces).
	// Independent of `scopes`.
	ProjectAware bool `gorm:"default:false" json:"projectAware"`

	// Visibility — who can see this space in the public catalog.
	// "public": anyone (default)
	// "org":    only members of OwnerOrgID; oracle staff still review.
	// Enforced both by a check constraint (visibility='org' ⟺ owner_org_id IS NOT NULL)
	// and by the listing handler that filters non-public rows by caller org.
	Visibility string `gorm:"size:20;not null;default:'public';index" json:"visibility"`
	// OwnerOrgID — set when the space was published by an org. For
	// visibility='org' this is the only org that can list/install it. For
	// visibility='public' this is metadata only (display: "by Acme Org").
	OwnerOrgID *string `gorm:"size:100;index" json:"owner_org_id,omitempty"`

	// Publisher attribution (denormalized for read perf)
	PublisherSlug string `gorm:"index"               json:"publisher_slug"`
	PublisherName string `                            json:"publisher_name"`

	// Categorization
	CategorySlug *string        `gorm:"index"       json:"category"`
	Tags         pq.StringArray `gorm:"type:text[]" json:"tags"`

	// Counters (denormalized; install_events is the source of truth)
	Downloads   int64 `gorm:"default:0"   json:"downloads"`
	Installs7d  int64 `gorm:"default:0"   json:"installs_7d"`
	Installs30d int64 `gorm:"default:0"   json:"installs_30d"`

	// Lifecycle
	PromotedAt time.Time  `gorm:"not null"  json:"promoted_at"`
	UpdatedAt  time.Time  `                  json:"updated_at"`
	DemotedAt  *time.Time `gorm:"index"     json:"demoted_at,omitempty"`
}

func (Space) TableName() string { return "spaces" }
