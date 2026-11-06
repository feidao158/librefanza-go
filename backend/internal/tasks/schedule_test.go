package tasks

import (
	"reflect"
	"testing"
	"time"
)

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
