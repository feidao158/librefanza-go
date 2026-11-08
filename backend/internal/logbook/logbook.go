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
	DetailsJSON  string    `json:"details_json,omitempty" gorm:"column:details_json"`
	Message      string    `json:"message,omitempty" gorm:"-"`
	ReasonText   string    `json:"reason_text,omitempty" gorm:"-"`
}

// RecommendationDetails 仅保存可用于汇总调优的非身份信息。
type RecommendationDetails struct {
	Algorithm  string `json:"algorithm,omitempty"`
	Gorse      string `json:"gorse,omitempty"`
	Origin     string `json:"origin,omitempty"`
	Action     string `json:"action,omitempty"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
	Candidates int    `json:"candidates,omitempty"`
	Returned   int    `json:"returned,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Liked      int    `json:"liked,omitempty"`
	Disliked   int    `json:"disliked,omitempty"`
	Uncertain  int    `json:"uncertain,omitempty"`
}

func RecommendationEntry(event, level string, details RecommendationDetails) Entry {
	data, _ := json.Marshal(details)
	return Entry{Level: level, Event: event, Source: "recommendation", DetailsJSON: string(data)}
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
	"recommendation_served": "推荐结果已生成", "recommendation_failed": "推荐结果生成失败", "recommendation_feedback": "推荐反馈已保存",
	"recommendation_sync_failed": "推荐数据同步失败，稍后重试", "recommendation_sync_resumed": "推荐数据同步已恢复",
}

var reasonMessages = map[string]string{
	"unknown": "原因未知", "http": "来源返回错误状态", "dns": "来源域名解析失败", "timeout": "请求超时",
	"connection_refused": "连接被拒绝", "connection_reset": "连接被重置", "tls": "TLS 连接或证书错误",
	"network": "网络请求失败", "cancelled": "任务已取消或服务正在停止", "verification": "来源要求验证",
	"configuration": "来源配置缺失或无效", "fanza_credentials": "未配置 Fanza API 凭据", "mgstage_series": "MGStage 系列列表地址未配置或无效",
	"proxy_configuration": "代理地址无效", "source_session": "来源会话无效", "invalid_response": "来源响应无法解析", "storage": "数据库操作失败", "panic": "任务执行期间发生异常",
	"source_retired": "来源已停用", "source_deferred": "来源暂时冷却",
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
	var save *sources.SaveError
	if errors.As(err, &save) {
		entry.Reason = "storage"
		return
	}
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
	case errors.Is(err, sources.ErrSourceRetired):
		entry.Reason = "source_retired"
	case err != nil && strings.Contains(err.Error(), "source temporarily deferred"):
		entry.Reason = "source_deferred"
	case err != nil && strings.Contains(err.Error(), "Fanza credentials are not configured"):
		entry.Reason = "fanza_credentials"
	case err != nil && (strings.Contains(err.Error(), "MGStage series URL is not configured") || strings.Contains(err.Error(), "invalid MGStage series URL")):
		entry.Reason = "mgstage_series"
	case err != nil && strings.Contains(err.Error(), "invalid proxy URL"):
		entry.Reason = "proxy_configuration"
	case err != nil && strings.Contains(err.Error(), "invalid source session"):
		entry.Reason = "source_session"
	case err != nil && strings.Contains(err.Error(), "not configured"):
		entry.Reason = "configuration"
	case err != nil && (strings.Contains(err.Error(), "cannot connect to database") || strings.Contains(err.Error(), "database connection test failed") || strings.Contains(err.Error(), "database migration failed")):
		entry.Reason = "storage"
	case err != nil && (strings.Contains(err.Error(), "invalid source") || strings.Contains(err.Error(), "invalid Fanza") || strings.Contains(err.Error(), "source page has no valid product") || strings.Contains(err.Error(), "source response too large")):
		entry.Reason = "invalid_response"
	}
}

func sanitize(entry *Entry) {
	entry.Level = allowed(entry.Level, "info", "warn", "error")
	entry.Event = allowed(entry.Event, "task_started", "task_completed", "task_failed", "source_failed", "worker_stopped", "worker_started", "worker_start_failed", "log_cleanup_failed", "recommendation_served", "recommendation_failed", "recommendation_feedback", "recommendation_sync_failed", "recommendation_sync_resumed")
	entry.Source = allowed(entry.Source, "fanza", "sod", "mgstage", "fc2", "jable", "avmoo", "javlibrary", "libredmm", "javdb", "javbus", "javbooks", "javquery", "recommendation")
	entry.TaskType = allowed(entry.TaskType, "movie_search", "source_crawl", "actress_crawl", "fanza_crawl", "mgstage_crawl", "rebuild_movies", "maintenance_tick", "metadata_enrich", "metadata_backfill", "metadata_backfill_tick", "metadata_source_retry", "source_history_backfill", "source_probe")
	entry.Reason = allowed(entry.Reason, "unknown", "http", "dns", "timeout", "connection_refused", "connection_reset", "tls", "network", "cancelled", "verification", "configuration", "fanza_credentials", "mgstage_series", "proxy_configuration", "source_session", "invalid_response", "storage", "panic", "source_retired", "source_deferred")
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
	if entry.Source != "recommendation" || len(entry.DetailsJSON) > 2048 {
		entry.DetailsJSON = ""
	} else if entry.DetailsJSON != "" {
		var details RecommendationDetails
		if json.Unmarshal([]byte(entry.DetailsJSON), &details) != nil {
			entry.DetailsJSON = ""
		} else {
			details.Algorithm = allowed(details.Algorithm, "gorse", "local", "not_ready")
			details.Gorse = allowed(details.Gorse, "disabled", "used", "empty", "error", "syncing")
			details.Origin = allowed(details.Origin, "annotation", "recommendation")
			details.Action = allowed(details.Action, "like", "dislike", "uncertain")
			details.Page = max(details.Page, 0)
			details.PageSize = max(details.PageSize, 0)
			details.Candidates = max(details.Candidates, 0)
			details.Returned = max(details.Returned, 0)
			details.DurationMS = max(details.DurationMS, 0)
			details.Liked = max(details.Liked, 0)
			details.Disliked = max(details.Disliked, 0)
			details.Uncertain = max(details.Uncertain, 0)
			data, _ := json.Marshal(details)
			entry.DetailsJSON = string(data)
		}
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
