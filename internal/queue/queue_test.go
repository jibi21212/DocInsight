package queue

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewQueue(t *testing.T) {
	q := NewQueue(10)
	if q == nil {
		t.Fatal("expected non-nil queue")
	}
	if q.Len() != 0 {
		t.Errorf("new queue length = %d, want 0", q.Len())
	}
}

func TestEnqueueAndDequeue(t *testing.T) {
	q := NewQueue(5)

	docID := uuid.New()
	job := NewProcessJob(docID, 3)

	if err := q.Enqueue(job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	if q.Len() != 1 {
		t.Errorf("queue length = %d, want 1", q.Len())
	}

	// Read from channel
	received := <-q.Jobs()
	if received.DocumentID != docID {
		t.Errorf("received.DocumentID = %v, want %v", received.DocumentID, docID)
	}
	if received.Type != JobTypeProcess {
		t.Errorf("received.Type = %v, want %v", received.Type, JobTypeProcess)
	}
}

func TestEnqueueFull(t *testing.T) {
	q := NewQueue(1)

	job1 := NewProcessJob(uuid.New(), 3)
	job2 := NewProcessJob(uuid.New(), 3)

	if err := q.Enqueue(job1); err != nil {
		t.Fatalf("first enqueue should succeed: %v", err)
	}

	err := q.Enqueue(job2)
	if err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestQueueClose(t *testing.T) {
	q := NewQueue(5)
	q.Close()

	// Reading from closed channel returns zero value and false
	_, ok := <-q.Jobs()
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestQueueDoubleClose(t *testing.T) {
	q := NewQueue(5)

	// Should not panic on double close
	q.Close()
	q.Close()
}

func TestNewProcessJob(t *testing.T) {
	docID := uuid.New()
	job := NewProcessJob(docID, 3)

	if job.DocumentID != docID {
		t.Errorf("DocumentID = %v, want %v", job.DocumentID, docID)
	}
	if job.Type != JobTypeProcess {
		t.Errorf("Type = %v, want %v", job.Type, JobTypeProcess)
	}
	if job.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", job.Attempts)
	}
	if job.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", job.MaxRetries)
	}
	if job.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestMultipleEnqueueDequeue(t *testing.T) {
	q := NewQueue(10)

	ids := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		ids[i] = uuid.New()
		job := NewProcessJob(ids[i], 3)
		if err := q.Enqueue(job); err != nil {
			t.Fatalf("enqueue %d failed: %v", i, err)
		}
	}

	if q.Len() != 5 {
		t.Errorf("queue length = %d, want 5", q.Len())
	}

	// Dequeue all in order (FIFO)
	for i := 0; i < 5; i++ {
		received := <-q.Jobs()
		if received.DocumentID != ids[i] {
			t.Errorf("dequeue %d: DocumentID = %v, want %v", i, received.DocumentID, ids[i])
		}
	}

	if q.Len() != 0 {
		t.Errorf("queue should be empty, length = %d", q.Len())
	}
}
