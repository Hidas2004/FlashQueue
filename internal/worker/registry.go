package worker

import (
	"context"
	"fmt"
	"job-scheduler/internal/job"
)

type JobHandler interface {
	Handler(ctx context.Context, j *job.Job) error
}

type Registry struct {
	//key là job type, value là handler tương ứng
	handlers map[job.Type]JobHandler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[job.Type]JobHandler),
	}
}

// job đến từ queue, dựa vào type của job để tìm handler tương ứng trong registry và gọi handler đó để xử lý job
func (r *Registry) Register(jobType job.Type, handler JobHandler) {
	r.handlers[jobType] = handler
}

func (r *Registry) Get(jobType job.Type) (JobHandler, error) {
	handler, exists := r.handlers[jobType]
	if !exists {
		return nil, fmt.Errorf("không tìm thấy handler cho job type: %s", jobType)
	}
	return handler, nil
}
