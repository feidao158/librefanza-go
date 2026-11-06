package sources

import (
	"context"
	"errors"

	"github.com/libredmm/librefanza/backend/internal/catalog"
)

type Provider interface {
	Search(context.Context, string) ([]Parsed, error)
}

type Sink interface {
	Save(context.Context, Parsed) error
}

func Search(ctx context.Context, keyword string, providers []Provider, sink Sink, observers ...func(string, error)) (bool, error) {
	for _, char := range keyword {
		if char > 127 {
			return false, errors.New("keyword must be ASCII")
		}
	}
	if !catalog.IsProductID(keyword) {
		return false, errors.New("invalid product ID")
	}
	id := catalog.NormalizeProductID(keyword)
	var failures []error
	for _, provider := range providers {
		items, err := provider.Search(ctx, id)
		if err != nil {
			failures = append(failures, err)
			if named, ok := provider.(interface{ SourceName() string }); ok {
				for _, observe := range observers {
					observe(named.SourceName(), err)
				}
			}
			continue
		}
		found := false
		for _, item := range items {
			if err := sink.Save(ctx, item); err != nil {
				return false, err
			}
			found = found || item.Item.NormalizedID == id
		}
		if found {
			return true, nil
		}
	}
	return false, errors.Join(failures...)
}
