package models

import (
	"encoding/json"
	"time"
)

// Collection is a curated grouping of spaces. App-Store-style
// "Staff Picks", "Editor's Choice", "Work From Home" — staff-authored
// in oracle-web, read-only public via /api/marketplace/collections.
//
// Two kinds:
//
//	"manual"  — operator picks the spaces and orders them. Spaces live
//	            in collection_spaces with a position.
//	"dynamic" — query-driven (sort by installs_7d, by category, etc.).
//	            Query is a small jsonb describing the filter+sort.
type Collection struct {
	ID         string          `gorm:"primaryKey;type:uuid"     json:"id"`
	Slug       string          `gorm:"uniqueIndex;size:64"      json:"slug"`         // 'staff-picks', 'trending'
	Title      string          `gorm:"not null"                 json:"title"`
	Subtitle   string          `gorm:""                         json:"subtitle"`
	HeroImage  string          `gorm:""                         json:"hero_image_url"`
	Kind       string          `gorm:"not null;index"           json:"kind"`         // 'manual' | 'dynamic'
	Query      json.RawMessage `gorm:"type:jsonb"               json:"query,omitempty"` // dynamic only
	Priority   int             `gorm:"default:100;index"        json:"priority"`     // home-page order
	ActiveFrom *time.Time      `gorm:"index"                    json:"active_from,omitempty"`
	ActiveTo   *time.Time      `gorm:"index"                    json:"active_to,omitempty"`
	CreatedBy  string          `gorm:"size:64"                  json:"created_by"`   // oracle staff user id
	CreatedAt  time.Time       `                                 json:"created_at"`
	UpdatedAt  time.Time       `                                 json:"updated_at"`
}

func (Collection) TableName() string { return "collections" }

// CollectionSpace is the manual-order join. Position is 1-indexed; lower
// shows first. Ignored when the parent Collection.Kind == "dynamic".
type CollectionSpace struct {
	CollectionID string    `gorm:"primaryKey;type:uuid"    json:"collection_id"`
	SpaceName    string    `gorm:"primaryKey;size:100"     json:"space_id"`     // matches spaces.name
	Position     int       `gorm:"not null;index"          json:"position"`
	AddedAt      time.Time `                                 json:"added_at"`
}

func (CollectionSpace) TableName() string { return "collection_spaces" }
