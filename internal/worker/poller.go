package worker

import (
	"context"
	"log"
	"time"

	"job-scheduler/internal/job"
	"job-scheduler/internal/queue"
)

// RetryPoller là một background service chuyên đi lùng sục DB
// để lôi các Job tới hẹn Retry nhét lại vào hàng đợi.
type RetryPoller struct {
	repo      *job.Repository
	publisher queue.Publisher // Sử dụng Interface thay vì Implementation
	interval  time.Duration
}

// NewRetryPoller Khởi tạo Poller. Inject (Tiêm) các thành phần từ bên ngoài vào.
func NewRetryPoller(repo *job.Repository, pub queue.Publisher, intervalSec int) *RetryPoller {
	return &RetryPoller{
		repo:      repo,
		publisher: pub,
		interval:  time.Duration(intervalSec) * time.Second,
	}
}

// Start Chạy vòng lặp vô tận (Daemon) để rình job
func (p *RetryPoller) Start(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	log.Printf("Retry poller đã khởi động. Tần suất dò tìm: %v", p.interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("Retry poller đã nhận lệnh dừng")
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// poll thực thi logic lõi của 1 chu kỳ dò rà
func (p *RetryPoller) poll(ctx context.Context) {
	// Lấy tối đa x jobs tới hẹn retry (Bên file DB dùng FOR UPDATE SKIP LOCKED rất chuẩn rồi)
	jobs, err := p.repo.GetDueRetryJobs(ctx)
	if err != nil {
		log.Printf("[Poller] Lỗi truy vấn DB: %v", err)
		return
	}

	if len(jobs) == 0 {
		// Kho không có gì, im lặng rút lui chờ chu kỳ kế tiếp
		return
	}

	log.Printf("[Poller] Phát hiện %d Jobs đã tới giờ chạy lại (Retry)", len(jobs))

	// Duyệt và bỏ lại lên băng chuyền
	for _, j := range jobs {

		if ctx.Err() != nil {
			log.Println("[Poller] Dừng Push vào Queue do hệ thống yêu cầu huỷ (Context Canceled)")
			return
		}

		msg := job.JobMessage{
			JobID: j.ID,
			Type:  job.Type(j.Type),
		}

		// Bắn vào hàng đợi. Ta cấp 1 timeout siêu ngắn (như 3s)
		// để RabbitMQ có chết cũng không làm kẹt Poller.
		pubCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := p.publisher.Publish(pubCtx, msg)
		cancel()

		if err != nil {
			// Nhỏ giọt lỗi (không Return), job nào lỗi mặc kệ, push tiếp job khác
			// Vì DB của cái job này vẫn đánh dấu là 'queued' ở hàm GetDueRetryJobs nên
			// nếu ta fail, nó sẽ lơ lửng, cần có cơ chế sweep lại ở DB sau này (ngoài phạm vi bài này).
			log.Printf("[Poller] Thất bại khi ném Job %s vào Queue: %v", j.ID, err)
			continue
		}

		log.Printf("[Poller] Đã ném Job %s thành công vào Queue để xử lý lại", j.ID)
	}
}
