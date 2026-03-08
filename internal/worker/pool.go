package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"job-scheduler/internal/job"
	"job-scheduler/internal/metrics"
	"job-scheduler/internal/queue"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

type Pool struct {
	workerCount   int
	repo          *job.Repository
	queue         *queue.RabbitMQ
	publisher     queue.Publisher
	registry      *Registry
	metrics       metrics.Metrics
	retryDelaySec int
	wg            sync.WaitGroup //đếm số lượng worker đang chạy
}

func NewPool(workerCount int, repo *job.Repository, q *queue.RabbitMQ, pub queue.Publisher, reg *Registry, m metrics.Metrics, retryDelaySec int) *Pool {
	return &Pool{
		workerCount:   workerCount,
		repo:          repo,
		queue:         q,
		publisher:     pub,
		registry:      reg,
		metrics:       m,
		retryDelaySec: retryDelaySec,
	}
}

func (p *Pool) Start(ctx context.Context) error {
	log.Printf("Starting %d workers", p.workerCount)
	for i := 0; i < p.workerCount; i++ { // tạo workerCount goroutine để xử lý job song song ,lặp đúng số lượng worker đã định nghĩa

		consumer, err := queue.NewConSumer(p.queue)
		if err != nil {
			return fmt.Errorf("worker %d: lỗi tạo consumer: %w", i+1, err)
		}

		deliveries, err := consumer.Consume()
		if err != nil {
			return fmt.Errorf("worker %d: lỗi lấy message stream: %w", i+1, err)
		}

		p.wg.Add(1)
		go p.runWorker(ctx, i+1, consumer, deliveries) // chạy worker trong goroutine riêng
	}
	<-ctx.Done()
	log.Println("Đang chờ các worker hoàn thành công việc hiện tại (Graceful Shutdown)...")
	p.wg.Wait()
	log.Println("Tất cả worker đã dừng an toàn.")
	return nil
}

// deliveries      <-chan        amqp.Delivery
// tên biến      loại channel    kiểu dữ liệu bên trong
// chan T là một channel có thể gửi và nhận dữ liệu kiểu T
// <-chan T là một channel chỉ có thể nhận dữ liệu kiểu T (read-only channel)
// chan<- T là một channel chỉ có thể gửi dữ liệu kiểu T (write-only channel)
// deliveries là một channel,Channel này chỉ được nhận dữ liệu (receive-only),Dữ liệu bên trong là amqp.Delivery
func (p *Pool) runWorker(ctx context.Context, id int, consumer *queue.RabbitConsumer, deliveries <-chan queue.Message) {
	defer p.wg.Done()
	defer consumer.Close()
	log.Printf("worker %d đã khởi động", id)
	for {
		//select Chờ nhiều channel cùng lúc.ai đến trước sẽ được xử lý trước
		select {
		case <-ctx.Done():
			log.Printf("worker %d: nhận tín hiện shutdowwn", id)
			return
		case delivery, ok := <-deliveries:
			if !ok {
				log.Printf("Worker %d: channel bị đóng từ phía RabbitMQ", id)
				return
			}
			p.processDeliverySafe(ctx, id, delivery)
		}
	}
}

func (p *Pool) processDeliverySafe(ctx context.Context, workerID int, delivery queue.Message) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("CRITICAL: Worker %d bị PANIC: %v\nStack trace: %s", workerID, r, string(debug.Stack()))
			delivery.Nack(false) // Nack để thông báo message không thể xử lý, cho bay vào DLX
		}
	}()
	p.processDelivery(ctx, workerID, delivery)
}

// processDeliverySafe bọc logic xử lý lại với Panic Recovery
func (p *Pool) processDelivery(ctx context.Context, workerID int, delivery queue.Message) {
	var msg job.JobMessage
	if err := json.Unmarshal(delivery.Body(), &msg); err != nil {
		log.Printf("Worker %d: sai định dạng message: %v", workerID, err)
		delivery.Nack(false) // Nack(requeue=false) để loại bỏ rác
		return
	}
	// 1. Khóa Job trong DB để đảm bảo không bị xử lý trùng
	j, err := p.repo.ClaimJob(ctx, msg.JobID)
	if err != nil {
		// Job đã có worker khác làm hoặc bị xóa
		delivery.Ack()
		return
	}
	log.Printf("Worker %d: Bắt đầu xử lý Job %s (type: %s, retry: %d)", workerID, j.ID, j.Type, j.RetryCount)
	// 2. Tra cứu Registry để lấy Handler (đúng chuẩn Clean Architecture)
	handler, err := p.registry.Get(job.Type(j.Type))
	if err != nil {
		p.handleExecutionError(ctx, j, delivery, workerID, err)
		return
	}
	// 3. Thực thi logic xử lý
	startTime := time.Now()
	// Cấp timeout cố định 30s cho mỗi Job, tránh Job chạy treo vĩnh viễn
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = handler.Handler(jobCtx, j)
	duration := time.Since(startTime).Seconds()
	// 4. Cập nhật kết quả
	if err == nil {
		p.repo.MarkCompleted(ctx, j.ID)
		p.metrics.IncJobProcessed(string(j.Type), "completed")
		p.metrics.ObserveJobDuration(string(j.Type), duration)
		log.Printf("Worker %d: Job %s HOÀN THÀNH trong %.2fs", workerID, j.ID, duration)
		delivery.Ack()
		return
	}
	p.handleExecutionError(ctx, j, delivery, workerID, err)
}

func (p *Pool) handleExecutionError(ctx context.Context, j *job.Job, delivery queue.Message, workerID int, execErr error) {
	if j.CanRetry() {
		log.Printf("Worker %d: Job %s THẤT BẠI (sẽ retry). Lỗi: %v", workerID, j.ID, execErr)
		p.repo.MarkFailed(ctx, j.ID, execErr.Error(), &time.Time{})
		p.metrics.IncJobRetry(string(j.Type))
		p.metrics.IncJobProcessed(string(j.Type), "failed")
		delivery.Ack()
		return
	}
	log.Printf("Worker %d: Job %s DEAD (hết lượt retry). Lỗi: %v", workerID, j.ID, execErr)
	p.repo.MarkFailed(ctx, j.ID, execErr.Error(), nil)
	p.metrics.IncJobProcessed(string(j.Type), "dead")
	// Publish thông tin vào Dead Letter Queue cho team Support điều tra
	dlqMsg := map[string]interface{}{
		"job_id":    j.ID,
		"type":      j.Type,
		"error":     execErr.Error(),
		"failed_at": time.Now(),
	}
	if err := p.publisher.PublishToDLQ(ctx, dlqMsg); err != nil {
		log.Printf("Worker %d: không thể publish job %s vào DLQ: %v", workerID, j.ID, err)
	}
	delivery.Ack()
}
