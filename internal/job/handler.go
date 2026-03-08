package job

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateJob(c *gin.Context) {
	var req CreateJobRequest

	// Trách nhiệm 1: Parse và Validate JSON từ body HTTP request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "dữ liệu đầu vào không hợp lệ",
			"details": err.Error(),
		})
		return
	}
	// Trách nhiệm 2: Chuyển dữ liệu xuống tầng Service
	j, err := h.service.CreateJob(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "lỗi hệ thống khi khởi tạo job",
			"details": err.Error(),
		})
		return
	}
	// Trách nhiệm 3: Trả về kết quả cho Client với mã 201 Created (Đã tạo thành công)
	c.JSON(http.StatusCreated, j)
}

func (h *Handler) GetJob(c *gin.Context) {
	id := c.Param("id") // Trích xuất giá trị {id} trên URL (vd: /jobs/123)
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "thiếu ID của job"})
		return
	}
	j, err := h.service.GetJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "không tìm thấy job hoặc job không tồn tại",
			"details": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, j)
}

func (h *Handler) ListJobs(c *gin.Context) {
	// Lấy tham số (query parameters) trên URL (vd: /jobs?status=pending&limit=10)
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	jobs, total, err := h.service.ListJobs(c.Request.Context(), status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "không thể lấy danh sách job", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":  jobs,
		"total": total,
	})
}
