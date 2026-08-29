package models

import "time"

// PersistentFile stores a filesystem asset as fixed-size chunks. It mirrors
// the mutable ./data assets that cannot be represented by relational models
// (themes, plugins, plugin storage, favicon, font, and GeoIP data).
type PersistentFile struct {
	Path      string                `json:"path" gorm:"type:varchar(512);primaryKey"`
	Mode      uint32                `json:"mode" gorm:"not null"`
	Size      int64                 `json:"size" gorm:"not null"`
	SHA256    string                `json:"sha256" gorm:"type:char(64);not null"`
	ChunkSize int                   `json:"chunk_size" gorm:"not null"`
	UpdatedAt time.Time             `json:"updated_at" gorm:"not null"`
	Chunks    []PersistentFileChunk `json:"-" gorm:"foreignKey:Path;references:Path;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
}

type PersistentFileChunk struct {
	Path  string `json:"path" gorm:"type:varchar(512);primaryKey"`
	Index uint32 `json:"index" gorm:"column:chunk_index;primaryKey"`
	Data  []byte `json:"-" gorm:"type:longblob;not null"`
}
