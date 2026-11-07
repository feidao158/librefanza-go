package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/robfig/cron/v3"
)

type Schedule struct {
	Enabled bool   `json:"enabled"`
	Cron    string `json:"cron"`
	Auto    bool   `json:"auto"`
}

type ScheduleSettings struct {
	Timezone   string   `json:"timezone"`
	Actress    Schedule `json:"actress"`
	Fanza      Schedule `json:"fanza"`
	MGStage    Schedule `json:"mgstage"`
	Jable      Schedule `json:"jable"`
	AVMOO      Schedule `json:"avmoo"`
	JavLibrary Schedule `json:"javlibrary"`
	LibreDMM   Schedule `json:"libredmm"`
	JavDB      Schedule `json:"javdb"`
	JavBus     Schedule `json:"javbus"`
	JavBooks   Schedule `json:"javbooks"`
	JavQuery   Schedule `json:"javquery"`
}

func DefaultScheduleSettings() ScheduleSettings {
	return ScheduleSettings{Timezone: "America/Los_Angeles", Actress: Schedule{Enabled: true, Cron: "0 0 * * *"}, Fanza: Schedule{Enabled: true, Cron: "*/30 * * * *"}, MGStage: Schedule{Enabled: true, Cron: "*/30 * * * *"}, Jable: Schedule{Cron: "0 3 * * *"}, LibreDMM: Schedule{Cron: "0 6 * * *"}, JavDB: Schedule{Cron: "*/30 * * * *", Auto: true}, JavBus: Schedule{Cron: "*/30 * * * *", Auto: true}, JavBooks: Schedule{Cron: "*/30 * * * *", Auto: true}, JavQuery: Schedule{Cron: "*/30 * * * *", Auto: true}}
}

func (s *ScheduleSettings) NewSourceSchedule(source string) *Schedule {
	switch source {
	case "javdb":
		return &s.JavDB
	case "javbooks":
		return &s.JavBooks
	case "javquery":
		return &s.JavQuery
	default:
		return nil
	}
}

func MergeScheduleSettings(previous ScheduleSettings, input []byte) (ScheduleSettings, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil {
		return ScheduleSettings{}, errors.New("invalid schedule settings")
	}
	merged := previous
	if err := json.Unmarshal(input, &merged); err != nil {
		return ScheduleSettings{}, err
	}
	for _, source := range []string{"javdb", "javbooks", "javquery"} {
		raw, exists := fields[source]
		if !exists {
			continue
		}
		var scheduleFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &scheduleFields); err != nil || scheduleFields == nil {
			return ScheduleSettings{}, errors.New("invalid schedule settings")
		}
		_, hasAuto := scheduleFields["auto"]
		_, hasEnabled := scheduleFields["enabled"]
		if !hasAuto && hasEnabled && previous.NewSourceSchedule(source).Enabled != merged.NewSourceSchedule(source).Enabled {
			merged.NewSourceSchedule(source).Auto = false
		}
	}
	return merged, nil
}

func DueSchedules(settings ScheduleSettings, now time.Time) ([]string, error) {
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return nil, errors.New("invalid schedule timezone")
	}
	var due []string
	for _, entry := range []struct {
		name     string
		schedule Schedule
	}{{"actress_crawl", settings.Actress}, {"fanza_crawl", settings.Fanza}, {"mgstage_crawl", settings.MGStage}, {"jable_crawl", settings.Jable}, {"libredmm_crawl", settings.LibreDMM}, {"javdb_crawl", settings.JavDB}, {"javbooks_crawl", settings.JavBooks}, {"javquery_crawl", settings.JavQuery}} {
		if !entry.schedule.Enabled {
			continue
		}
		parsed, err := cron.ParseStandard("CRON_TZ=" + location.String() + " " + entry.schedule.Cron)
		if err != nil {
			return nil, fmt.Errorf("invalid schedule: %s", entry.name)
		}
		boundary := now.Truncate(time.Minute)
		if !parsed.Next(boundary.Add(-time.Second)).After(boundary) {
			due = append(due, entry.name)
		}
	}
	return due, nil
}

func ValidateScheduleSettings(settings ScheduleSettings) error {
	_, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return errors.New("invalid schedule timezone")
	}
	for _, value := range []Schedule{settings.Actress, settings.Fanza, settings.MGStage, settings.Jable, settings.LibreDMM, settings.JavDB, settings.JavBooks, settings.JavQuery} {
		if !value.Enabled && value.Cron == "" {
			continue
		}
		if _, err := cron.ParseStandard(value.Cron); err != nil {
			return errors.New("invalid schedule expression")
		}
	}
	return nil
}

func NextSchedules(settings ScheduleSettings, now time.Time) (map[string]time.Time, error) {
	if err := ValidateScheduleSettings(settings); err != nil {
		return nil, err
	}
	location, _ := time.LoadLocation(settings.Timezone)
	next := make(map[string]time.Time)
	for _, entry := range []struct {
		name     string
		schedule Schedule
	}{
		{"actress", settings.Actress}, {"fanza", settings.Fanza}, {"mgstage", settings.MGStage},
		{"jable", settings.Jable}, {"libredmm", settings.LibreDMM}, {"javdb", settings.JavDB}, {"javbooks", settings.JavBooks}, {"javquery", settings.JavQuery},
	} {
		if !entry.schedule.Enabled {
			continue
		}
		parsed, err := cron.ParseStandard("CRON_TZ=" + location.String() + " " + entry.schedule.Cron)
		if err != nil {
			return nil, err
		}
		next[entry.name] = parsed.Next(now).In(location)
	}
	return next, nil
}

func (m *Manager) ConfigureMaintenance(load func(context.Context) (ScheduleSettings, error), handlers map[string]func(context.Context) error) error {
	for _, name := range []string{"actress_crawl", "fanza_crawl", "mgstage_crawl", "rebuild_movies"} {
		if handlers[name] == nil {
			return errors.New("missing maintenance handler")
		}
	}
	for name, handler := range handlers {
		handler := handler
		m.queue.Register(name, func(ctx context.Context, _ struct{}) error {
			return protectTaskError(func() error {
				err := handler(ctx)
				var deferred *sources.DeferredError
				if errors.As(err, &deferred) {
					return nil
				}
				return err
			})
		})
	}
	m.queue.Register("maintenance_tick", func(ctx context.Context, _ struct{}) error {
		return protectTaskError(func() error {
			settings, err := load(ctx)
			if err != nil {
				return err
			}
			due, err := DueSchedules(settings, time.Now())
			if err != nil {
				return err
			}
			for _, name := range due {
				if name == "jable_crawl" || name == "libredmm_crawl" || name == "javdb_crawl" || name == "javbus_crawl" || name == "javbooks_crawl" || name == "javquery_crawl" {
					_, err := m.EnqueueCrawl(ctx, sources.CrawlRequest{Source: strings.TrimSuffix(name, "_crawl"), Mode: "incremental"})
					if err != nil && !errors.Is(err, ErrCrawlInProgress) {
						return err
					}
					continue
				}
				_, err := m.queue.Enqueue(ctx, name, struct{}{}, jobs.Unique("maintenance:"+name), jobs.Retries(3), jobs.Timeout(4*time.Hour))
				if err != nil && !errors.Is(err, jobs.ErrDuplicateJob) {
					return err
				}
			}
			return nil
		})
	})
	schedule, err := jobs.Cron("* * * * *")
	if err != nil {
		return err
	}
	return m.queue.Schedule("maintenance_tick", struct{}{}, schedule, jobs.Unique("maintenance_tick"))
}

func (m *Manager) EnqueueMaintenance(ctx context.Context, name string) (string, error) {
	switch name {
	case "actress_crawl", "fanza_crawl", "mgstage_crawl", "rebuild_movies":
	default:
		return "", errors.New("invalid maintenance task")
	}
	id, err := m.queue.Enqueue(ctx, name, struct{}{}, jobs.Unique("maintenance:"+name), jobs.Retries(3), jobs.Timeout(4*time.Hour))
	if errors.Is(err, jobs.ErrDuplicateJob) {
		var existing jobs.Job
		err = m.db.WithContext(ctx).Where("unique_key = ? AND status IN ?", "maintenance:"+name, []string{"pending", "running", "retrying", "waiting", "paused"}).Order("created_at DESC").First(&existing).Error
		if err == nil {
			return existing.ID.String(), nil
		}
	}
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
