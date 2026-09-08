package models

import "time"

// EditorialOverride lets staff override publisher-supplied descriptive
// fields on a per-space basis without touching the source manifest.
// One row per space; nil columns mean "use publisher value".
//
// App Store parallel: an editor's "Featured app" hero copy that's
// distinct from the publisher's own description.
type EditorialOverride struct {
	SpaceName string    `gorm:"primaryKey;size:100"       json:"space_id"`              // matches spaces.name
	Blurb     *string   `                                  json:"blurb,omitempty"`
	HeroImage *string   `                                  json:"hero_image_url,omitempty"`
	Title     *string   `                                  json:"title,omitempty"`
	Pinned    bool      `gorm:"default:false;index"       json:"pinned"`
	UpdatedBy string    `gorm:"size:64"                   json:"updated_by"`
	UpdatedAt time.Time `                                  json:"updated_at"`
}

func (EditorialOverride) TableName() string { return "editorial_overrides" }
