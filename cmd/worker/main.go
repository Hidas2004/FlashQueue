package main

import (
	"context"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"job-scheduler/internal/config"
	"job-scheduler/internal/db"
	"job-scheduler/internal/job"
	"job-scheduler/internal/metrics"
	"job-scheduler/internal/queue"
	"job-scheduler/internal/worker"
)

func main() {
	// Cấu hình Logger
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()
	defer func() { _ = logger.Sync() }() // Fix lỗi syscall stdout

	sugar.Info("Tiến trình Worker đang khởi động...")

	cfg := config.Load()

	// Khởi tạo Dependency
	database := db.NewPostgres(cfg.DatabaseURL)
	defer database.Close()

	mq, err := queue.NewRabbitMQFromURL(cfg.RabbitMQURL, cfg.QueueName, cfg.DLQName)
	if err != nil {
		sugar.Fatalf("Lỗi kết nối RabbitMQ: %v", err)
	}
	defer mq.Close()

	repo := job.NewRepository(database)

	pub, err := queue.NewPublisher(mq)
	if err != nil {
		sugar.Fatalf("Lỗi tạo RabbitMQ Publisher: %v", err)
	}

	registry := worker.NewRegistry()
	met := metrics.NewPrometheusMetrics()

	// Khởi tạo Worker Pool & Poller
	pool := worker.NewPool(cfg.WorkerCount, repo, mq, pub, registry, met, cfg.RetryDelay)
	poller := worker.NewRetryPoller(repo, pub, 10) // Quét delay job mỗi 10 giây

	// Tạo Root Context cho Worker
	// Dùng cấu trúc signal.NotifyContext tiện lợi hơn Signal thuần có từ Go 1.16+
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel() // Tự do dọn dẹp khi exit function block

	// Chạy Poller quét Job Retry dưới nền (Nó cũng nhận ctx để tự dừng khi có lệnh)
	go poller.Start(ctx)

	// Chạy Worker Pool CHÍNH - Chặn lại chờ tới khi xong (Blocking)
	// Hàm pool.Start(ctx) bên dưới TỰ HIỂU:
	// "À tao nhận thấy ctx bị HỦY, tao sẽ không lấy Job mới nữa, tao báo tất cả các Worker đang làm ráng làm xong nốt job dở dang rồi tắt!"
	sugar.Infof("Worker Pool khởi động với %d workers 💪", cfg.WorkerCount)
	if err := pool.Start(ctx); err != nil {
		sugar.Errorf("Worker Pool kết thúc với cảnh báo lỗi: %v", err)
	}

	// Đoạn này ta có thể làm thêm một bước ShutdownTimeout cứng để đề phòng
	// Worker hỏng không chịu nhả Ctx (VD: Bị DeadLock hàm bên trong).
	// Hiện tại pool.Start đã dùng sync.WaitGroup (p.wg.Wait()) nên khá ổn.

	sugar.Info("Toàn bộ tiến trình Worker đã dừng an toàn.")
}
