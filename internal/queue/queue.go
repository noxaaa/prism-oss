package queue

import (
	"context"
	"time"
)

type Job struct {
	Type           string
	Payload        []byte
	IdempotencyKey string
	Queue          string
	Priority       int
}

type FailedJob struct {
	Job       Job
	Error     string
	FailedAt  time.Time
	Attempts  int
	Exhausted bool
}

type Queue interface {
	Enqueue(ctx context.Context, job Job) error
	Schedule(ctx context.Context, job Job, runAt time.Time) error
	FailedJobs(ctx context.Context, queue string) ([]FailedJob, error)
}
