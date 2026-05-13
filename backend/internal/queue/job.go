package queue

import (
	"time"

	"github.com/google/uuid"
)

type JobType string

const (
	JobTypeProcess JobType = "process_document"
)

type Job struct {
	ID         string
	Type       JobType
	DocumentID uuid.UUID
	CreatedAt  time.Time
	Attempts   int
	MaxRetries int
}

func NewProcessJob(documentID uuid.UUID, maxRetries int) Job {
	return Job{
		ID:         uuid.New().String(),
		Type:       JobTypeProcess,
		DocumentID: documentID,
		CreatedAt:  time.Now(),
		Attempts:   0,
		MaxRetries: maxRetries,
	}
}
