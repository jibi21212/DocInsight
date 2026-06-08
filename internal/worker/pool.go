package worker

import (
	"context"
	"log/slog"
	"sync"

	"github.com/docinsight/backend/internal/queue"
)

type Pool struct {
	workers   int
	queue     *queue.Queue
	processor *Processor
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewPool(workers int, q *queue.Queue, proc *Processor) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		workers:   workers,
		queue:     q,
		processor: proc,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (p *Pool) Start() {
	slog.Info("starting worker pool", "workers", p.workers)
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.runWorker(i)
	}
}

func (p *Pool) runWorker(id int) {
	defer p.wg.Done()
	slog.Info("worker started", "worker_id", id)

	for {
		select {
		case <-p.ctx.Done():
			slog.Info("worker shutting down", "worker_id", id)
			return
		case job, ok := <-p.queue.Jobs():
			if !ok {
				slog.Info("worker channel closed", "worker_id", id)
				return
			}
			slog.Info("worker processing job",
				"worker_id", id,
				"job_id", job.ID,
				"document_id", job.DocumentID,
				"attempt", job.Attempts,
			)
			p.processor.Process(p.ctx, job)
		}
	}
}

func (p *Pool) Shutdown() {
	slog.Info("shutting down worker pool...")
	p.cancel()
	p.queue.Close()
	p.wg.Wait()
	slog.Info("worker pool shut down")
}
