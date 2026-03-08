package job

import (
	"context"
	"fmt"
	"job-scheduler/internal/queue"
)

type RepositoryInterface interface {
	Create(ctx context.Context, req *CreateJobRequest) (*Job, error)
	List(ctx context.Context, status string, limit, offset int) ([]*Job, int, error)
	GetByID(ctx context.Context, id string) (*Job, error)
}

type Service struct {
	repo      RepositoryInterface
	publisher queue.Publisher
}

func NewService(repo RepositoryInterface, publisher queue.Publisher) *Service {
	return &Service{
		repo:      repo,
		publisher: publisher,
	}
}

func (s *Service) CreateJob(ctx context.Context, req *CreateJobRequest) (*Job, error) {
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}
	if req.Priority < 0 {
		req.Priority = 0
	}
	j, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("không thể khởi tạo job trong DB: %w", err)
	}
	//đóng gói message và đẩy vào hàng đợi Queue để worker xử lý
	msg := JobMessage{JobID: j.ID, Type: Type(j.Type)}
	//  Đã commit vào CSDL nhưng bắn qua Queue thất bại!
	if err := s.publisher.Publish(ctx, msg); err != nil {
		return j, fmt.Errorf("cảnh báo: lưu DB thành công nhưng push lên RabbitMQ thất bại: %w", err)
	}
	return j, nil
}

func (s *Service) GetJob(ctx context.Context, id string) (*Job, error) {
	return s.repo.GetByID(ctx, id)
}
func (s *Service) ListJobs(ctx context.Context, status string, limit, offset int) ([]*Job, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, status, limit, offset)
}
