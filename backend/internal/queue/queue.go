package queue

import (
	"errors"
	"log/slog"
	"sync"
)

var ErrQueueFull = errors.New("job queue is full")

type Queue struct {
	jobs     chan Job
	closeOnce sync.Once
}

func NewQueue(capacity int) *Queue {
	slog.Info("job queue created", "capacity", capacity)
	return &Queue{
		jobs: make(chan Job, capacity),
	}
}

func (q *Queue) Enqueue(job Job) error {
	select {
	case q.jobs <- job:
		slog.Info("job enqueued", "job_id", job.ID, "document_id", job.DocumentID, "attempt", job.Attempts)
		return nil
	default:
		return ErrQueueFull
	}
}

func (q *Queue) Jobs() <-chan Job {
	return q.jobs
}

func (q *Queue) Len() int {
	return len(q.jobs)
}

func (q *Queue) Close() {
	q.closeOnce.Do(func() { close(q.jobs) })
}
