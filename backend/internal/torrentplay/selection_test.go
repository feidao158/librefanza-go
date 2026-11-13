package torrentplay

import "testing"

func TestSelectVideoPrefersBrowserCompatibleLargest(t *testing.T) {
	files := []videoFile{{"sample.mp4", 5}, {"movie.mkv", 5000}, {"main.MP4", 1000}, {"video.webm", 200}}
	if n := selectVideo(files); n != 2 {
		t.Fatalf("选择错误: %d", n)
	}
	if n := selectVideo([]videoFile{{"unknown.exe", 100}}); n != -1 {
		t.Fatalf("不应选择未知格式: %d", n)
	}
}
