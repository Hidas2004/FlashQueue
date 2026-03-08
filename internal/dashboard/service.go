package dashboard

import (
	"context"
)

// RepositoryInterface (Dependency Inversion)
type RepositoryInterface interface {
	GetStats(ctx context.Context) (map[string]int, error)
}

// Service cấu trúc đóng gói logic nghiệp vụ
type Service struct {
	repo RepositoryInterface
}

func NewService(repo RepositoryInterface) *Service {
	return &Service{repo: repo}
}

// GetDashboardStats là nơi chứa "trí tuệ" của chức năng.
func (s *Service) GetDashboardStats(ctx context.Context) (map[string]int, error) {
	// 1. Lấy dữ liệu thô từ Database
	stats, err := s.repo.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Business Logic: Xử lý cộng tổng số lượng Job
	total := 0
	for _, count := range stats {
		total += count
	}
	stats["total"] = total

	// 3. Business Logic: Đảm bảo Dashboard luôn nhận đủ các trạng thái cốt lõi
	coreStatuses := []string{"pending", "running", "completed", "failed", "retrying", "dead"}
	for _, st := range coreStatuses {
		if _, exists := stats[st]; !exists {
			stats[st] = 0 // Điền mặc định = 0
		}
	}

	return stats, nil
}
