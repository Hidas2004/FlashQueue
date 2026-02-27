package db

import (
	"context"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func NewPostgres(dsn string) *sqlx.DB {
	//1 phân tích dsn
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("CRITICAL: Failed to parse PostgreSQL DSN: %v", err)
	}
	// 2. Thiết lập bộ 3 tham số của Connection Pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(15 * time.Minute)
	// 3. Ping thử tới DB với Timeout rõ ràng
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("CRITICAL: Failed to connect to PostgreSQL: %v", err)
	}
	log.Println("INFO: Successfully connected to PostgreSQL")
	return db
}
