package models

type OidcProvider struct {
	Name     string `json:"name" gorm:"primaryKey;type:varchar(255);unique;not null"`
	Addition string `json:"addition" gorm:"type:longtext" default:"{}"`
}
