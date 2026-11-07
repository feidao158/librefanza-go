package recommendation

import (
	"context"

	gorse "github.com/gorse-io/gorse-go"
)

// GorseClientInterface 定义 Gorse 客户端接口，便于单元测试与扩展
type GorseClientInterface interface {
	InsertUser(ctx context.Context, user gorse.User) (gorse.RowAffected, error)
	InsertItem(ctx context.Context, item gorse.Item) (gorse.RowAffected, error)
	InsertFeedback(ctx context.Context, feedbacks []gorse.Feedback) (gorse.RowAffected, error)
	GetRecommend(ctx context.Context, userId string, category string, n, offset int) ([]string, error)
}

// defaultGorseClient 基于官方 gorse-go SDK 的实现
type defaultGorseClient struct {
	client *gorse.GorseClient
}

// NewGorseClient 创建 Gorse 客户端封装
func NewGorseClient(entryPoint, apiKey string) GorseClientInterface {
	return &defaultGorseClient{
		client: gorse.NewGorseClient(entryPoint, apiKey),
	}
}

func (c *defaultGorseClient) InsertUser(ctx context.Context, user gorse.User) (gorse.RowAffected, error) {
	return c.client.InsertUser(ctx, user)
}

func (c *defaultGorseClient) InsertItem(ctx context.Context, item gorse.Item) (gorse.RowAffected, error) {
	return c.client.InsertItem(ctx, item)
}

func (c *defaultGorseClient) InsertFeedback(ctx context.Context, feedbacks []gorse.Feedback) (gorse.RowAffected, error) {
	return c.client.InsertFeedback(ctx, feedbacks)
}

func (c *defaultGorseClient) GetRecommend(ctx context.Context, userId string, category string, n, offset int) ([]string, error) {
	return c.client.GetRecommend(ctx, userId, category, n, offset)
}
