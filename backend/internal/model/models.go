package model

import "time"

type Dashboard struct {
	ID              uint      `json:"id"`
	UID             string    `json:"uid" gorm:"uniqueIndex;size:64"`
	Name            string    `json:"name" gorm:"size:120;not null"`
	Description     string    `json:"description" gorm:"size:500"`
	TimeRange       string    `json:"timeRange" gorm:"size:40;not null"`
	RefreshInterval int       `json:"refreshInterval" gorm:"not null;default:30"`
	Revision        int       `json:"revision" gorm:"not null;default:1"`
	Panels          []Panel   `json:"panels" gorm:"foreignKey:DashboardID;constraint:OnDelete:CASCADE;"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Panel struct {
	ID                uint   `json:"id"`
	DashboardID       uint   `json:"-"`
	UID               string `json:"uid" gorm:"size:64;index"`
	Title             string `json:"title" gorm:"size:120;not null"`
	Type              string `json:"type" gorm:"size:40;not null"`
	PosX              int    `json:"x" gorm:"column:pos_x"`
	PosY              int    `json:"y" gorm:"column:pos_y"`
	PosW              int    `json:"w" gorm:"column:pos_w"`
	PosH              int    `json:"h" gorm:"column:pos_h"`
	Query             string `json:"query" gorm:"type:text"`
	QueryConfigJSON   string `json:"-" gorm:"type:json"`
	OptionsJSON       string `json:"-" gorm:"type:json"`
	VisualizationJSON string `json:"-" gorm:"type:json"`
}

// DataSource stores non-secret connection settings separately from encrypted credentials.
// The configuration JSON never contains passwords, tokens, cookies, or authorization headers.
type DataSource struct {
	ID         uint      `json:"id"`
	UID        string    `json:"uid" gorm:"uniqueIndex;size:64"`
	Name       string    `json:"name" gorm:"size:120;not null"`
	Type       string    `json:"type" gorm:"size:30;not null"`
	ConfigJSON string    `json:"-" gorm:"type:json"`
	SecretJSON string    `json:"-" gorm:"type:longtext"`
	Status     string    `json:"status" gorm:"size:30;not null;default:'unknown'"`
	LastError  string    `json:"lastError,omitempty" gorm:"size:500"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
