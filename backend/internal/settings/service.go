package settings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/mailer"
	"github.com/libredmm/librefanza/backend/internal/setup"
	"github.com/libredmm/librefanza/backend/internal/sourcehealth"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/libredmm/librefanza/backend/internal/tasks"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

type setting struct {
	Name      string `gorm:"primaryKey"`
	Value     string
	UpdatedAt time.Time
}

func (s *Service) load(ctx context.Context, name string, value any) (bool, error) {
	var row setting
	err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(row.Value), value); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) save(ctx context.Context, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	row := setting{Name: name, Value: string(data), UpdatedAt: time.Now().UTC()}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"})}).Create(&row).Error
}

func (s *Service) Sources(ctx context.Context) (sources.ClientSettings, error) {
	result := sources.ClientSettings{FanzaAPIID: os.Getenv("FANZA_API_ID"), FanzaAffiliateID: os.Getenv("FANZA_AFFILIATE_ID"), ProxyURL: os.Getenv("PROXY_URL"), FC2URL: os.Getenv("FC2_BASE_URL"), MGStageSeriesURL: os.Getenv("MGSTAGE_SERIES_URL")}
	_, err := s.load(ctx, "sources", &result)
	result.RequestLimiter = database.NewSourceRateLimiter(s.db)
	result.Health = sourcehealth.New(s.db)
	return result, err
}

func (s *Service) SourceHealth(ctx context.Context) ([]sourcehealth.State, error) {
	return sourcehealth.New(s.db).List(ctx)
}

func (s *Service) UpdateSources(ctx context.Context, input sources.ClientSettings) error {
	previous, err := s.Sources(ctx)
	if err != nil {
		return err
	}
	if input.FanzaAPIID == "" {
		input.FanzaAPIID = previous.FanzaAPIID
	}
	if input.FanzaAffiliateID == "" {
		input.FanzaAffiliateID = previous.FanzaAffiliateID
	}
	if input.ClearProxy {
		input.ProxyURL = ""
	} else if input.ProxyURL == "" {
		input.ProxyURL = previous.ProxyURL
	}
	input.ClearProxy = false
	if input.ClearJableCookie {
		input.JableCookie = ""
	} else if input.JableCookie == "" {
		input.JableCookie = previous.JableCookie
	}
	if input.ClearJavLibraryCookie {
		input.JavLibraryCookie = ""
	} else if input.JavLibraryCookie == "" {
		input.JavLibraryCookie = previous.JavLibraryCookie
	}
	input.ClearJableCookie, input.ClearJavLibraryCookie = false, false
	for _, entry := range []struct {
		current  *string
		previous string
		clear    *bool
	}{
		{&input.JavDBCookie, previous.JavDBCookie, &input.ClearJavDBCookie},
		{&input.JavBusCookie, previous.JavBusCookie, &input.ClearJavBusCookie},
		{&input.JavBooksCookie, previous.JavBooksCookie, &input.ClearJavBooksCookie},
		{&input.JavQueryCookie, previous.JavQueryCookie, &input.ClearJavQueryCookie},
	} {
		if *entry.clear {
			*entry.current = ""
		} else if *entry.current == "" {
			*entry.current = entry.previous
		}
		*entry.clear = false
	}
	if _, err := sources.NewProviders(input); err != nil {
		return err
	}
	return s.save(ctx, "sources", input)
}

func (s *Service) Redis(ctx context.Context) (config.Redis, error) {
	var result config.Redis
	_, err := s.load(ctx, "redis", &result)
	return result, err
}

func (s *Service) UpdateRedis(ctx context.Context, input config.Redis) error {
	previous, err := s.Redis(ctx)
	if err != nil {
		return err
	}
	if input.ClearPassword {
		input.Password = ""
	} else if input.Password == "" {
		input.Password = previous.Password
	}
	input.ClearPassword = false
	if input.Address == "" {
		input.Address = previous.Address
	}
	if input.Enabled {
		if err := setup.NewDatabaseBackend().CheckRedis(ctx, input); err != nil {
			return err
		}
	}
	return s.save(ctx, "redis", input)
}

func (s *Service) Schedules(ctx context.Context) (tasks.ScheduleSettings, error) {
	result := tasks.DefaultScheduleSettings()
	_, err := s.load(ctx, "schedules", &result)
	if err != nil {
		return result, err
	}
	states, err := sourcehealth.New(s.db).List(ctx)
	if err != nil {
		return result, err
	}
	for _, state := range states {
		if schedule := result.NewSourceSchedule(state.Source); schedule != nil && schedule.Auto && state.Verified {
			schedule.Enabled = true
		}
	}
	return result, nil
}

func (s *Service) UpdateSchedules(ctx context.Context, input tasks.ScheduleSettings) error {
	if err := tasks.ValidateScheduleSettings(input); err != nil {
		return err
	}
	return s.save(ctx, "schedules", input)
}

func (s *Service) SMTP(ctx context.Context) (mailer.Settings, error) {
	var result mailer.Settings
	_, err := s.load(ctx, "smtp", &result)
	return result, err
}

func (s *Service) UpdateSMTP(ctx context.Context, input mailer.Settings) error {
	previous, err := s.SMTP(ctx)
	if err != nil {
		return err
	}
	if input.Password == "" {
		input.Password = previous.Password
	}
	if err := mailer.Validate(input); err != nil {
		return err
	}
	return s.save(ctx, "smtp", input)
}

// GorseSettings Gorse 推荐系统配置
type GorseSettings struct {
	EntryPoint       string `json:"entry_point"`
	APIKey           string `json:"api_key"`
	Enabled          bool   `json:"enabled"`
	MinFeedbackCount int    `json:"min_feedback_count"`
}

// Gorse 读取 Gorse 推荐配置
func (s *Service) Gorse(ctx context.Context) (GorseSettings, error) {
	result := GorseSettings{
		EntryPoint:       os.Getenv("GORSE_ENTRY_POINT"),
		APIKey:           os.Getenv("GORSE_API_KEY"),
		Enabled:          os.Getenv("GORSE_ENTRY_POINT") != "",
		MinFeedbackCount: 5,
	}
	_, err := s.load(ctx, "gorse", &result)
	if result.MinFeedbackCount <= 0 {
		result.MinFeedbackCount = 5
	}
	return result, err
}

// UpdateGorse 更新 Gorse 推荐配置
func (s *Service) UpdateGorse(ctx context.Context, input GorseSettings) error {
	previous, err := s.Gorse(ctx)
	if err != nil {
		return err
	}
	if input.APIKey == "" {
		input.APIKey = previous.APIKey
	}
	if input.EntryPoint == "" {
		input.EntryPoint = previous.EntryPoint
	}
	if input.MinFeedbackCount <= 0 {
		input.MinFeedbackCount = 5
	}
	return s.save(ctx, "gorse", input)
}
