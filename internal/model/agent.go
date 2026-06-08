package model

import "time"

type Agent struct {
	ID uint `gorm:"primaryKey"`

	AgentName string `gorm:"uniqueIndex;not null"`
	Hostname  string
	Version   string
	Status    string

	LastSeenAt     time.Time
	LastSequence   int64
	LastRenewToken string

	CreatedAt time.Time
	UpdatedAt time.Time
}
