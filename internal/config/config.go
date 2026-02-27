package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config chứa toàn bộ cấu hình lõi của ứng dụng
type Config struct {
	ServerPort  string
	MetricsPort string
	DatabaseURL string
	RabbitMQURL string
	WorkerCount int
	QueueName   string
	DLQName     string
	RetryDelay  int
}

// Load sẽ đọc các biến môi trường và trả về struct config
// nếu có cấu hình nào sai hệ thống sẽ panic luôn
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Info: No .env file found, reading strictly from environment variables")
	}
	return &Config{
		ServerPort:  getEnvString("SERVER_PORT", "8080"),
		MetricsPort: getEnvString("METRICS_PORT", "8081"),
		DatabaseURL: getEnvString("DATABASE_URL", "postgres://jobuser:jobpass@localhost:5432/jobscheduler?sslmode=disable"),
		RabbitMQURL: getEnvString("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		QueueName:   getEnvString("QUEUE_NAME", "jobs"),
		DLQName:     getEnvString("DLQ_NAME", "jobs.dlq"),
		WorkerCount: getEnvInt("WORKER_COUNT", 5),
		RetryDelay:  getEnvInt("RETRY_DELAY_SECONDS", 5),
	}
}

// getEnvString đọc biến môi trường kiểu String
func getEnvString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// getEnvInt đọc và ép kiểu an toàn biến môi trường sang Integer (Chống lỗi Crash ngầm)
func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}

	intValue, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("CRITICAL: Invalid value for environment variable %s. Expected integer, got: '%s'", key, v)
	}
	return intValue
}
