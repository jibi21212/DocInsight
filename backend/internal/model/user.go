package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	APIKey    string    `json:"api_key,omitempty"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
