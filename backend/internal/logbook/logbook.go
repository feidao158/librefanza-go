package logbook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/libredmm/librefanza/backend/internal/sources"
	"gorm.io/gorm"
)

type Entry struct {
	ID           uint64    `json:"id" gorm:"primaryKey"`
	CreatedAt    time.Time `json:"created_at"`
	Level        string    `json:"level"`
	Event        string    `json:"event"`
	Source       string    `json:"source"`
	TaskType     string    `json:"task_type"`
	TaskID       string    `json:"task_id"`
	Attempt      int       `json:"attempt"`
	Reason       string    `json:"reason"`
	HTTPStatus   int       `json:"http_status"`
	Stage        string    `json:"stage"`
	PositionType string    `json:"position_type"`
	Position     int       `json:"position"`
	Message      string    `json:"message,omitempty" gorm:"-"`
	ReasonText   string    `json:"reason_text,omitempty" gorm:"-"`
}

func (Entry) TableName() string { return "application_logs" }

var ErrTaskPanic = errors.New("task handler panicked")

type Recorder struct {
	db     *gorm.DB
	output io.Writer
	mu     sync.Mutex
}

func New(db *gorm.DB, output io.Writer) *Recorder { return &Recorder{db: db, output: output} }

func (r *Recorder) write(entry Entry) {
	if r.output == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = json.NewEncoder(r.output).Encode(entry)
}

func (r *Recorder) Record(ctx context.Context, entry Entry) {
	entry.ID = 0
	entry.CreatedAt = time.Now().UTC()
	sanitize(&entry)
	entry.Message = eventMessages[entry.Event]
	entry.ReasonText = reasonMessages[entry.Reason]
	r.write(entry)
	if r.db == nil {
		return
	}
	writeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := r.db.WithContext(writeContext).Create(&entry).Error; err != nil {
		r.write(Entry{CreatedAt: time.Now().UTC(), Level: "error", Event: "log_storage_failed", Reason: "storage", Message: "日志写入数据库失败", ReasonText: reasonMessages["storage"]})
	}
}

var eventMessages = map[string]string{
	"task_started": "任务开始执行", "task_completed": "任务执行完成", "task_failed": "任务执行失败",
	"source_failed": "来源检索失败", "worker_stopped": "后台任务处理器意外停止", "worker_started": "后台任务处理器已恢复",
	"worker_start_failed": "后台任务处理器启动失败", "log_cleanup_failed": "日志清理失败",
}

var reasonMessages = map[string]string{
	"unknown": "原因未知", "http": "来源返回错误状态", "dns": "来源域名解析失败", "timeout": "请求超时",
	"connection_refused": "连接被拒绝", "connection_reset": "连接被重置", "tls": "TLS 连接或证书错误",
	"network": "网络请求失败", "cancelled": "任务已取消或服务正在停止", "verification": "来源要求验证",
	"configuration": "来源配置缺失或无效", "invalid_response": "来源响应无法解析", "storage": "数据库操作失败", "panic": "任务执行期间发生异常",
}

func (r *Recorder) Clean(ctx context.Context, before time.Time) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&Entry{}).Error
}

type Filter struct {
	Level    string
	Source   string
	TaskID   string
	Page     int
	PageSize int
}

func List(ctx context.Context, db *gorm.DB, filter Filter) ([]Entry, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 30
	}
	query := db.WithContext(ctx).Model(&Entry{})
	if filter.Level != "" {
		query = query.Where("level = ?", filter.Level)
	}
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}
	if filter.TaskID != "" {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []Entry
	err := query.Order("id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&entries).Error
	return entries, total, err
}

func ApplyFailure(entry *Entry, err error) {
	entry.Reason, entry.HTTPStatus, entry.Stage, entry.PositionType, entry.Position = "unknown", 0, "", "", 0
	var crawl *sources.CrawlError
	if errors.As(err, &crawl) {
		entry.Stage, entry.PositionType, entry.Position = crawl.Stage, crawl.PositionType, crawl.Position
		if crawl.Stage == "save" || crawl.Stage == "progress" {
			entry.Reason = "storage"
			return
		}
	}
	var request *sources.RequestError
	if errors.As(err, &request) {
		entry.Reason, entry.HTTPStatus = request.Reason, request.Status
		return
	}
	switch {
	case errors.Is(err, ErrTaskPanic):
		entry.Reason = "panic"
	case errors.Is(err, context.Canceled):
		entry.Reason = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		entry.Reason = "timeout"
	case err != nil && strings.Contains(err.Error(), "source requires verification"):
		entry.Reason = "verification"
	case err != nil && (strings.Contains(err.Error(), "not configured") || strings.Contains(err.Error(), "invalid proxy URL")):
		entry.Reason = "configuration"
	case err != nil && (strings.Contains(err.Error(), "invalid source") || strings.Contains(err.Error(), "invalid Fanza") || strings.Contains(err.Error(), "source page has no valid product") || strings.Contains(err.Error(), "source response too large")):
		entry.Reason = "invalid_response"
	}
}

func sanitize(entry *Entry) {
	entry.Level = allowed(entry.Level, "info", "warn", "error")
	entry.Event = allowed(entry.Event, "task_started", "task_completed", "task_failed", "source_failed", "worker_stopped", "worker_started", "worker_start_failed", "log_cleanup_failed")
	entry.Source = allowed(entry.Source, "fanza", "sod", "mgstage", "fc2", "jable", "avmoo", "javlibrary", "libredmm")
	entry.TaskType = allowed(entry.TaskType, "movie_search", "source_crawl", "actress_crawl", "fanza_crawl", "mgstage_crawl", "rebuild_movies", "maintenance_tick")
	entry.Reason = allowed(entry.Reason, "unknown", "http", "dns", "timeout", "connection_refused", "connection_reset", "tls", "network", "cancelled", "verification", "configuration", "invalid_response", "storage", "panic")
	entry.Stage = allowed(entry.Stage, "fetch", "save", "progress")
	entry.PositionType = allowed(entry.PositionType, "page", "number")
	if len(entry.TaskID) != 36 || strings.Trim(entry.TaskID, "0123456789abcdefABCDEF-") != "" {
		entry.TaskID = ""
	}
	if entry.HTTPStatus < 100 || entry.HTTPStatus > 599 {
		entry.HTTPStatus = 0
	}
	if entry.Attempt < 0 {
		entry.Attempt = 0
	}
	if entry.Position < 0 {
		entry.Position = 0
	}
}

func allowed(value string, options ...string) string {
	for _, option := range options {
		if value == option {
			return value
		}
	}
	return ""
}
