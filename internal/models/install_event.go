package models

import "time"

// InstallEvent is one install ping from a desktop client. Insert-only;
// rolled up nightly into Space.Installs7d / Installs30d for fast trending
// queries. Raw rows kept for ~90 days then aged out.
type InstallEvent struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"   json:"id"`
	SpaceName string    `gorm:"index;size:100;not null"    json:"space_id"`            // matches spaces.name
	Version   string    `                                  json:"version"`
	UserID    string    `gorm:"index;size:64"              json:"user_id,omitempty"`
	OrgID     string    `gorm:"index;size:64"              json:"org_id,omitempty"`
	IPHash    string    `gorm:"size:64"                    json:"-"`
	Country   string    `gorm:"size:2"                     json:"country,omitempty"`
	CreatedAt time.Time `gorm:"index;not null"             json:"created_at"`
}

func (InstallEvent) TableName() string { return "install_events" }
