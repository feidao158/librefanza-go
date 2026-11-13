package torrentplay

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// 未完成分片在可响应读取中可能暂时返回 EOF，实际失败仍通过播放接口反馈。
type quietTransientReader struct{ slog.Handler }

func (h quietTransientReader) Handle(ctx context.Context, record slog.Record) error {
	if record.Message == "initial read failed" {
		transient := false
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "err" {
				if err, ok := attr.Value.Any().(error); ok && errors.Is(err, io.EOF) {
					transient = true
				}
			}
			return true
		})
		if transient {
			return nil
		}
	}
	return h.Handler.Handle(ctx, record)
}
func (h quietTransientReader) WithAttrs(attrs []slog.Attr) slog.Handler {
	return quietTransientReader{h.Handler.WithAttrs(attrs)}
}
func (h quietTransientReader) WithGroup(name string) slog.Handler {
	return quietTransientReader{h.Handler.WithGroup(name)}
}
