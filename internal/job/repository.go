package job

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrJobNotFound = errors.New("job not found or already claimed")

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// nhiều worker chạy song song , nhưng chỉ 1 worker được quyền sử lý 1 job
func (r *Repository) ClaimJob(ctx context.Context, jobID string) (*Job, error) {
	var j Job
	// Bắt đầu một Transaction (Giao dịch)
	//BeginTxx: mở 1 transaction gom nhiều câu lệnh sql lại với nhau (atomic)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // Đảm bảo rollback nếu có lỗi xảy ra
	// Câu lệnh SQL để claim job
	// Truy vấn kết hợp khóa row
	err = tx.GetContext(ctx, &j, `
		SELECT * FROM jobs
		WHERE id = $1 AND status IN ('pending', 'retrying')
		FOR UPDATE SKIP LOCKED
	`, jobID)
	if err != nil {
		//Xử lý trường hợp không tìm thấy job hoặc đã bị claim bởi worker khác
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	// Cập nhật trạng thái của job thành "running"
	_, err = tx.ExecContext(ctx, `
		UPDATE jobs SET status = 'running',started_at = NOW() WHERE id = $1
	`, jobID)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &j, nil
}

// Tạo mới một job
// và trả về job vừa được tạo ra (bao gồm ID và các trường khác)
func (r *Repository) Create(ctx context.Context, req *CreateJobRequest) (*Job, error) {
	var j Job
	err := r.db.GetContext(ctx, &j, `
	INSERT INTO jobs (type,payload,priority,max_retries,scheduled_at)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING *   // Trả về toàn bộ thông tin của job vừa được tạo ra, bao gồm ID và các trường khác
	`, req.Type, req.Payload, req.Priority, req.MaxRetries, req.ScheduledAt)
	return &j, err
}

// job bị lỗi , quyết định xem có retry hay chết và câp nhật lại thông tin của job trong DB
func (r *Repository) MarkFailed(ctx context.Context, id string, errMsg string, nextRetryAt *time.Time) error {
	var err error
	if nextRetryAt == nil {
		//dead luôn vì hết retry
		_, err = r.db.ExecContext(ctx, `
			UPDATE jobs SET status = 'dead',error_message = $1 WHERE id = $2
		`, errMsg, id)
	} else {
		_, err = r.db.ExecContext(ctx, `
			UPDATE jobs
			SET status = 'retrying',
				error_message = $1,
				next_retry_at = $2,
				retry_count = retry_count + 1
			WHERE id = $3
		`, errMsg, nextRetryAt, id)
	}
	return err
}

func (r *Repository) List(ctx context.Context, status string, limit, offset int) ([]*Job, int, error) {
	// Xây dựng điều kiện WHERE dựa trên tham số status (nếu có)
	whereClause := ""
	var args []interface{}
	if status != "" {
		whereClause = "WHERE status = $1"
		args = append(args, status)
	}
	//Đếm tổng số lượng (Để làm Phân trang)
	var total int
	countQuery := r.db.Rebind("SELECT COUNT(*) FROM jobs" + whereClause)
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}
	// Truy vấn danh sách job với phân trangvà sắp xếp theo priority và created_at
	query := "SELECT * FROM jobs " + whereClause +
		"ORDER BY priority DESC,created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	query = r.db.Rebind(query)
	var jobs []*Job
	if err := r.db.SelectContext(ctx, &jobs, query, args...); err != nil {
		return nil, 0, err
	}
	return jobs, total, nil
}

// hàm lấy job theo ID
func (r *Repository) GetByID(ctx context.Context, id string) (*Job, error) {
	var j Job
	err := r.db.GetContext(ctx, &j, `SELECT * FROM jobs WHERE id = $1`, id)
	return &j, err
}

// đánh dấu job đã hoàn thành/chết
func (r *Repository) MarkCompleted(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'completed', completed_at = NOW() WHERE id = $1
	`, id)
	return err
}

// thống kê số lượng job theo từng trạng thái
func (r *Repository) GetStats(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryxContext(ctx, `SELECT status, COUNT(*) as count FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	//Tạo map để chứa kết quả thống kê
	stats := map[string]int{}
	for rows.Next() {
		var s struct {
			Status string `db:"status"`
			Count  int    `db:"count"`
		}
		rows.StructScan(&s)
		stats[s.Status] = s.Count
	}
	return stats, nil
}

// Lấy Job Đến Hạn Retry
func (r *Repository) GetDueRetryJobs(ctx context.Context) ([]*Job, error) {
	var jobs []*Job
	query := `
		WITH locked_jobs AS (
			SELECT id FROM jobs
			WHERE status = 'retrying' AND next_retry_at <= NOW()
			ORDER BY priority DESC, next_retry_at ASC
			LIMIT 100
			FOR UPDATE SKIP LOCKED
		)
		UPDATE jobs
		SET status = 'queued' -- hoặc 1 mác gì đó đánh dấu
		FROM locked_jobs
		WHERE jobs.id = locked_jobs.id
		RETURNING jobs.*;
	`
	err := r.db.SelectContext(ctx, &jobs, query)
	return jobs, err
}
