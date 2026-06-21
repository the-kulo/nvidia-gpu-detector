package model

import "time"

const (
	SessionStatusOnline = "online"
	SessionStatusEnded  = "ended"
)

type AgentSession struct {
	ID uint `gorm:"primaryKey"`

	AgentName string `gorm:"index;not null"`
	SessionID string `gorm:"uniqueIndex;not null"`

	Hostname string
	Version  string

	Status string `gorm:"not null"`

	LastSequence   int64
	LastRenewToken string
	LastSeenAt     time.Time

	StartedAt time.Time
	EndedAt   time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}
