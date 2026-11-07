package runtime

import (
	"context"

	"github.com/libredmm/librefanza/backend/internal/sources"
)

type verifiedCrawlSink struct {
	sink     sources.Sink
	source   string
	mark     func(context.Context, string) error
	verified bool
}

func (s *verifiedCrawlSink) Save(ctx context.Context, item sources.Parsed) error {
	if err := s.sink.Save(ctx, item); err != nil {
		return err
	}
	if !s.verified {
		if err := s.mark(ctx, s.source); err != nil {
			return err
		}
		s.verified = true
	}
	return nil
}
