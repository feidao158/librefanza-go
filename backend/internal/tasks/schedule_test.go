package tasks

import (
	"reflect"
	"testing"
	"time"
)

func TestAdditionalSourceScheduleIsDue(t *testing.T) {
	settings := DefaultScheduleSettings()
	settings.Jable = Schedule{Enabled: true, Cron: "15 3 * * *"}
	settings.Timezone = "Asia/Shanghai"
	now := time.Date(2025, 1, 2, 3, 15, 0, 0, time.FixedZone("CST", 8*60*60))
	due, err := DueSchedules(settings, now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range due {
		found = found || name == "jable_crawl"
	}
	if !found {
		t.Fatalf("未触发 Jable 任务：%v", due)
	}
}

func TestNextScheduleUsesConfiguredTimezone(t *testing.T) {
	settings := DefaultScheduleSettings()
	settings.Timezone = "Asia/Shanghai"
	settings.Jable = Schedule{Enabled: true, Cron: "30 9 * * *"}
	now := time.Date(2025, 1, 2, 1, 0, 0, 0, time.UTC)
	next, err := NextSchedules(settings, now)
	if err != nil {
		t.Fatal(err)
	}
	if next["jable"].Format(time.RFC3339) != "2025-01-02T09:30:00+08:00" {
		t.Fatalf("下次执行时间 = %v", next["jable"])
	}
	if _, ok := next["avmoo"]; ok {
		t.Fatal("停用任务不应显示下次执行时间")
	}
}

func TestDueSchedulesUseConfiguredTimeZone(t *testing.T) {
	settings := DefaultScheduleSettings()
	when := time.Date(2026, 1, 10, 9, 0, 10, 0, time.UTC)
	got, err := DueSchedules(settings, when)
	if err != nil || !reflect.DeepEqual(got, []string{"fanza_crawl"}) {
		t.Fatalf("due=%v err=%v", got, err)
	}
	settings.Fanza.Enabled = false
	got, err = DueSchedules(settings, when)
	if err != nil || len(got) != 0 {
		t.Fatalf("disabled task due=%v err=%v", got, err)
	}
	settings.Timezone = "not/a-zone"
	if _, err := DueSchedules(settings, when); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}
