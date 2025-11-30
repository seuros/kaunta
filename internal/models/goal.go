package models

import "time"

// Goal represents a conversion goal
type Goal struct {
	ID          string    `json:"id" db:"id"`
	WebsiteID   string    `json:"website_id" db:"website_id"`
	Name        string    `json:"name" db:"name"`
	TargetURL   *string   `json:"target_url,omitempty" db:"target_url"`
	TargetEvent *string   `json:"target_event,omitempty" db:"target_event"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
