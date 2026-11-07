package sources

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveSourceDiagnostic(t *testing.T) {
	name := os.Getenv("LIBREFANZA_LIVE_DIAGNOSTIC")
	if name == "" {
		t.Skip("需要显式启用在线来源检查")
	}
	ids := map[string]string{"fanza": "ABP-123", "sod": "SDDE-772", "mgstage": "ARA-168", "fc2": "FC2-3069309", "jable": "SORA-655", "libredmm": "PPPE-460", "javdb": "ABP-123", "javbus": "ABP-123", "javbooks": "LUCY-032", "javquery": "SDDE-772"}
	id, ok := ids[name]
	if !ok {
		t.Fatal("未知来源")
	}
	settings := ClientSettings{FanzaAPIID: os.Getenv("FANZA_API_ID"), FanzaAffiliateID: os.Getenv("FANZA_AFFILIATE_ID"), ProxyURL: os.Getenv("PROXY_URL")}
	if name == "fanza" && (settings.FanzaAPIID == "" || settings.FanzaAffiliateID == "") {
		t.Skip("未配置 Fanza 凭据")
	}
	providers, err := NewProviders(settings)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, provider := range providers {
		if provider.(*httpProvider).kind != name {
			continue
		}
		items, err := provider.Search(ctx, id)
		if err != nil {
			t.Fatalf("来源检索失败: %v", err)
		}
		if len(items) == 0 {
			t.Fatal("没有找到验证番号")
		}
		item := items[0]
		t.Logf("来源=%s 番号匹配=%t 标题=%t 封面=%t 缩略图=%t 发行日期=%t", name, item.Item.NormalizedID == id, item.Item.Title != "", item.Item.CoverImageURL != "", item.Item.ThumbnailImageURL != "", item.Item.ReleaseDate != nil)
		return
	}
	t.Fatal("来源未注册")
}

func TestLiveNewSourceProbe(t *testing.T) {
	name := os.Getenv("LIBREFANZA_LIVE_PROBE")
	if name == "" {
		t.Skip("需要显式启用在线列表探测")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	item, err := ProbeSource(ctx, ClientSettings{ProxyURL: os.Getenv("PROXY_URL")}, name)
	if err != nil {
		t.Fatalf("来源列表或详情探测失败: %v", err)
	}
	t.Logf("来源=%s 番号=%t 标题=%t 封面=%t 日期=%t", name, item.Item.NormalizedID != "", item.Item.Title != "", item.Item.CoverImageURL != "", item.Item.ReleaseDate != nil)
}
