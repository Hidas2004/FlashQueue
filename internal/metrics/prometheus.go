package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var _ Metrics = (*prometheusMetrics)(nil)

type prometheusMetrics struct {
	jobsProcessedTotal    *prometheus.CounterVec
	jobProcessingDuration *prometheus.HistogramVec
	jobsRetryTotal        *prometheus.CounterVec
	queueDepth            prometheus.Gauge
}

func NewPrometheusMetrics() *prometheusMetrics {
	//opts (cấu hình)
	return &prometheusMetrics{
		jobsProcessedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "jobs_processed_total",
				Help: "Tổng số lượng jobs đã được xử lý (phân tách theo type và status)",
			},
			[]string{"type", "status"},
		),
		jobProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "job_processing_duration_seconds",
				Help:    "Thời gian (giây) tiêu tốn để xử lý một công việc hoàn chỉnh",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
			},
			[]string{"type"},
		),
		jobsRetryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "jobs_retry_total",
				Help: "Tổng số lần các job phải trigger trạng thái Retry",
			},
			[]string{"type"},
		), queueDepth: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "jobs_queue_depth",
				Help: "Sĩ số jobs hiện đang nhốt trong hàng đợi (Pending/Retrying)",
			},
		),
	}
}

func (m *prometheusMetrics) IncJobProcessed(jobType string, status string) {
	//Inc tăng giá trị lên
	m.jobsProcessedTotal.WithLabelValues(jobType, status).Inc()
}
func (m *prometheusMetrics) ObserveJobDuration(jobType string, durationSec float64) {
	m.jobProcessingDuration.WithLabelValues(jobType).Observe(durationSec)
}
func (m *prometheusMetrics) IncJobRetry(jobType string) {
	m.jobsRetryTotal.WithLabelValues(jobType).Inc()
}
func (m *prometheusMetrics) SetQueueDepth(depth float64) {
	m.queueDepth.Set(depth)
}
