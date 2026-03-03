package metrics

type Metrics interface {
	// Ghi nhận 1 job vừa xử lý xong với status thành công/thất bại
	IncJobProcessed(jobType string, status string)
	// Ghi nhận thời gian (duration) hoàn thành 1 job
	ObserveJobDuration(jobType string, durationSec float64)
	// Ghi nhận 1 job gặp lỗi và đang phải Retry
	IncJobRetry(jobType string)
	// Cập nhật số lượng job đang chờ (pending) trong hàng đợi (Gauge)
	SetQueueDepth(depth float64)
}
