package models

type ThemeFile struct {
	Short     string    `json:"short" gorm:"primaryKey;type:varchar(255)"`
	Data      []byte    `json:"-" gorm:"type:longblob"` // Store zip file content
	UpdatedAt LocalTime `json:"updated_at"`
}

type StaticFile struct {
	Name      string    `json:"name" gorm:"primaryKey;type:varchar(255)"`
	Data      []byte    `json:"-" gorm:"type:longblob"`
	UpdatedAt LocalTime `json:"updated_at"`
}
