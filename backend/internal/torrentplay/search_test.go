package torrentplay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchSelectsHighestSeededExactID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "START-639" || r.URL.Query().Get("s") != "seeders" {
			t.Error("搜索参数错误")
		}
		fmt.Fprint(w, `<table class="torrent-list"><tbody>
<tr><td></td><td><a href="/view/1" title="START-6390">START-6390</a></td><td><a href="magnet:?xt=urn:btih:1111111111111111111111111111111111111111">M</a></td><td>1 GiB</td><td></td><td>900</td><td>0</td><td>0</td></tr>
<tr><td></td><td><a href="/view/2" title="START-639 字幕">START-639</a></td><td><a href="magnet:?xt=urn:btih:2222222222222222222222222222222222222222">M</a></td><td>1 GiB</td><td></td><td>8</td><td>0</td><td>0</td></tr>
<tr><td></td><td><a href="/view/3" title="START-639">START-639</a></td><td><a href="magnet:?xt=urn:btih:3333333333333333333333333333333333333333">M</a></td><td>1 GiB</td><td></td><td>20</td><td>0</td><td>0</td></tr>
</tbody></table>`)
	}))
	defer server.Close()
	result, err := Search(context.Background(), server.Client(), server.URL, "START-639")
	if err != nil || len(result) != 2 || result[0].Seeders != 20 || result[1].Seeders != 8 {
		t.Fatalf("结果=%+v 错误=%v", result, err)
	}
}

func TestMagnetRetainsOnlyKnownTrackers(t *testing.T) {
	value := cleanMagnet("magnet:?xt=urn:btih:2222222222222222222222222222222222222222&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=http%3A%2F%2F127.0.0.1%2Fsecret")
	if !strings.Contains(value, "tracker.opentrackr.org") || strings.Contains(value, "127.0.0.1") {
		t.Fatal("Tracker 过滤失败")
	}
}
