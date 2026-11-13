package torrentplay

import (
	"path/filepath"
	"strings"
)

type videoFile struct {
	name string
	size int64
}

func selectVideo(files []videoFile) int {
	best := -1
	for i, file := range files {
		ext := strings.ToLower(filepath.Ext(file.name))
		if (ext != ".mp4" && ext != ".webm") || file.size <= 0 {
			continue
		}
		if best < 0 || file.size > files[best].size {
			best = i
		}
	}
	return best
}
