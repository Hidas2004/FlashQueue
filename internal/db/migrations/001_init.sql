CREATE TYPE job_status AS ENUM ('pending', 'running', 'completed', 'failed', 'retrying', 'dead');

--Cấu trúc lõi của bảng chứa Job
CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(100) NOT NULL,
    payload        JSONB NOT NULL DEFAULT '{}',
    status         job_status NOT NULL DEFAULT 'pending',
    --priority sự ưu tiên , worker bốc ra làm trước
    priority       INT NOT NULL DEFAULT 0,
    --có nguy cơ 2 Worker cùng bốc trúng 1 job, version để giải quyết vấn đề này
    version        INT NOT NULL DEFAULT 0,
    retry_count    INT NOT NULL DEFAULT 0,
    max_retries    INT NOT NULL DEFAULT 3,
    error_message   TEXT,
    next_retry_at  TIMESTAMP WITH TIME ZONE,
    scheduled_at   TIMESTAMP WITH TIME ZONE DEFAULT NOW(), -- TỐI ƯU: Default luôn có để dễ đánh query
    started_at     TIMESTAMP WITH TIME ZONE,
    completed_at   TIMESTAMP WITH TIME ZONE,
    created_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Tạo index để tối ưu truy vấn theo type và status
CREATE INDEX idx_jobs_pending_fetch
    ON jobs(status, scheduled_at, priority DESC, created_at)
    WHERE status = 'pending';--chỉ index những job đang pending để tối ưu truy vấn bốc job

-- 2. Index cho Retry Poller (Trình quét job cần thử lại)
CREATE INDEX idx_jobs_retrying_fetch
    ON jobs(next_retry_at) 
    WHERE status = 'retrying'; --chỉ index những job đang retrying (lỗi tạm thời) để tối ưu truy vấn quét job cần thử lại

-- 3. Index để dọn Zombie Jobs (Job đang running quá lâu)
CREATE INDEX idx_jobs_zombie_sweep 
    ON jobs(started_at) 
    WHERE status = 'running';

-- Trigger tự cập nhật updated_at khi có thay đổi
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_updated_at
    BEFORE UPDATE ON jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();