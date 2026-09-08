package models

import "time"

// Category is the App-Store-style top-level taxonomy. Lookup table —
// staff-managed via oracle-web. Slug is the stable id used in URLs and
// in the spaces.category_slug FK.
type Category struct {
	Slug        string    `gorm:"primaryKey;size:64"        json:"slug"`        // 'productivity', 'dev-tools', …
	Title       string    `gorm:"not null"                  json:"title"`
	Description string    `gorm:""                          json:"description"`
	Icon        string    `gorm:""                          json:"icon"`        // lucide name
	Position    int       `gorm:"default:100;index"         json:"position"`    // home-page sort
	Visible     bool      `gorm:"default:true"              json:"visible"`
	CreatedAt   time.Time `                                  json:"created_at"`
	UpdatedAt   time.Time `                                  json:"updated_at"`
}

func (Category) TableName() string { return "categories" }
