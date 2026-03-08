package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"job-scheduler/internal/config"
	"job-scheduler/internal/dashboard"
	"job-scheduler/internal/db"
	"job-scheduler/internal/job"
	"job-scheduler/internal/queue"
)

func main() {
	// 1. Khởi tạo Logger chuẩn
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()
	// Bỏ qua lỗi rác khi đóng stdout/stderr trên một số hệ điều hành
	defer func() { _ = logger.Sync() }()

	sugar.Info("Tiến trình API đang khởi động...")

	// 2. Load Cấu hình
	cfg := config.Load()

	// 3. Kết nối Database (Phải có xử lý timeout/retry, nhưng ở đây tạm mượn code cũ)
	database := db.NewPostgres(cfg.DatabaseURL)
	defer database.Close()
	sugar.Info("Kết nối PostgreSQL thành công")

	// 4. Kết nối RabbitMQ
	mq, err := queue.NewRabbitMQFromURL(cfg.RabbitMQURL, cfg.QueueName, cfg.DLQName)
	if err != nil {
		sugar.Fatalf("Lỗi kết nối RabbitMQ: %v", err)
	}
	defer mq.Close()
	sugar.Info("Kết nối RabbitMQ thành công")

	// 5. DEPENDENCY INJECTION (Bơm phụ thuộc) - QUAN TRỌNG NHẤT
	// -- Chuyên môn Core Job --
	repo := job.NewRepository(database)
	pub, err := queue.NewPublisher(mq)
	if err != nil {
		sugar.Fatalf("Lỗi tạo RabbitMQ Publisher: %v", err)
	}
	jobSvc := job.NewService(repo, pub)
	jobHandler := job.NewHandler(jobSvc)

	// -- Chuyên môn Dashboard (Update theo cấu trúc bài trước) --
	dashSvc := dashboard.NewService(repo)
	dashHandler := dashboard.NewHandler(dashSvc)

	// 6. Router & Middleware
	r := gin.New()
	r.Use(gin.Recovery())

	// (Nên chèn thêm Zap Logger Middleware ở đây thay cho gin.Logger mặc định)

	// CORS Setup (Lưu ý: Không dùng "*" trên Production bảo mật kém)
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*") // Hãy chỉ whitelist domain của bạn
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 7. Gắn Route (Routing)
	v1 := r.Group("/api/v1")
	v1.POST("/jobs", jobHandler.CreateJob)
	v1.GET("/jobs", jobHandler.ListJobs)
	v1.GET("/jobs/:id", jobHandler.GetJob)

	dash := r.Group("/api/dashboard")
	dash.GET("/stats", dashHandler.GetStats) // Nhận UI tĩnh hoặc API tĩnh

	// Metrics & Health
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	// 8. Cấu hình HTTP Server & Chạy nền
	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: r,
	}

	// 9. Graceful Shutdown Flow
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM) // Lắng nghe Ctrl+C (SIGINT) hoặc lệnh kill (SIGTERM)

	// Chạy HTTP API ở Goroutine phụ
	go func() {
		sugar.Infof("API Server đang chạy ở cổng :%s 🚀", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sugar.Fatalf("Lỗi Http Server: %v", err)
		}
	}()

	// Trương chình bị BLOCKED ở đây chờ nhận tín hiệu Quit
	<-quit
	sugar.Info("Nhận được tín hiệu ngắt. Bắt đầu tiến trình Graceful Shutdown API...")

	// Cho Server tối đa 10 giây để ráng xử lý nốt các HTTP req đang lỡ cỡ
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		sugar.Errorf("API Server bị ép tắt do lỗi/vượt quá 10s timeout: %v", err)
	}
	sugar.Info("API Server dừng an toàn.")
}
