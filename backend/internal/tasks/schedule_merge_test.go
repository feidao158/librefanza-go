package tasks

import (
	"testing"
	"time"
)

func TestRetiredJavBusIsNotScheduled(t *testing.T) {
	settings := DefaultScheduleSettings()
	settings.JavBus = Schedule{Enabled: true, Auto: true, Cron: "* * * * *"}
	due, err := DueSchedules(settings, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range due {
		if name == "javbus_crawl" {
			t.Fatal("已停用来源不应运行定时任务")
		}
	}
	next, err := NextSchedules(settings, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next["javbus"]; ok {
		t.Fatal("已停用来源不应出现下次执行时间")
	}
}

func TestMergeScheduleSettingsPreservesNewSourceDefaults(t *testing.T) {
	previous := DefaultScheduleSettings()
	previous.JavDB.Enabled = true
	merged, err := MergeScheduleSettings(previous, []byte(`{"timezone":"Asia/Shanghai","fanza":{"enabled":false,"cron":"*/30 * * * *"}}`))
	if err != nil || merged.Timezone != "Asia/Shanghai" || merged.Fanza.Enabled || !merged.JavDB.Auto || !merged.JavDB.Enabled {
		t.Fatalf("旧客户端覆盖了新增来源设置: %+v %v", merged, err)
	}
	merged, err = MergeScheduleSettings(previous, []byte(`{"javdb":{"enabled":false,"cron":"*/30 * * * *"}}`))
	if err != nil || merged.JavDB.Auto || merged.JavDB.Enabled {
		t.Fatalf("手动关闭未阻止自动启用: %+v %v", merged.JavDB, err)
	}
}
