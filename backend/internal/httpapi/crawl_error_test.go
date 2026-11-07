package httpapi

import "testing"

func TestRetiredSourceCrawlHasSpecificErrorCode(t *testing.T) {
	if code := crawlErrorCode("no retry: source retired"); code != "source_retired" {
		t.Fatalf("退役来源错误码 = %q", code)
	}
}
