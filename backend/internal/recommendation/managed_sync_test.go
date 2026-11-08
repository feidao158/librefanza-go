package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
)

type syncTestClient struct {
	fail     bool
	items    []gorse.Item
	feedback []gorse.Feedback
}

func TestManagedUserIDUsesStableKeyedHash(t *testing.T) {
	t.Setenv("GORSE_API_KEY", "test-key")
	first := managedUserID("user_123")
	if first == managedUserID("user_456") || first != managedUserID("user_123") || strings.Contains(first, "user_123") {
		t.Fatal("内部用户标识不符合预期")
	}
	t.Setenv("GORSE_API_KEY", "other-key")
	if first == managedUserID("user_123") {
		t.Fatal("不同密钥不能生成相同标识")
	}
}

func (c *syncTestClient) InsertItem(_ context.Context, item gorse.Item) (gorse.RowAffected, error) {
	if c.fail {
		return gorse.RowAffected{}, errors.New("unavailable")
	}
	c.items = append(c.items, item)
	return gorse.RowAffected{}, nil
}
func (c *syncTestClient) DeleteFeedbacks(_ context.Context, _, _ string) (gorse.RowAffected, error) {
	if c.fail {
		return gorse.RowAffected{}, errors.New("unavailable")
	}
	c.feedback = nil
	return gorse.RowAffected{}, nil
}
func (c *syncTestClient) InsertFeedback(_ context.Context, feedback []gorse.Feedback) (gorse.RowAffected, error) {
	if c.fail {
		return gorse.RowAffected{}, errors.New("unavailable")
	}
	c.feedback = append(c.feedback, feedback...)
	return gorse.RowAffected{}, nil
}

func TestManagedSyncRetriesAndResumes(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	id := "SYNC-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	movie := catalog.Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	feedback := UserFeedback{UserID: id, NormalizedID: id, FeedbackType: "like", Score: 1}
	if err := db.Create(&feedback).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", movie.ID).Delete(&catalog.Movie{})
	defer db.Where("user_id = ?", id).Delete(&UserFeedback{})
	defer db.Where("name = ?", "gorse_sync_test").Delete(&managedSyncState{})
	client := &syncTestClient{fail: true}
	syncer := newManagedSync(db, client, "gorse_sync_test")
	if err := syncer.step(context.Background()); err == nil {
		t.Fatal("同步失败未返回错误")
	}
	client.fail = false
	syncer = newManagedSync(db, client, "gorse_sync_test")
	if err := syncer.step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(client.items) == 0 || client.items[0].ItemId != id {
		t.Fatalf("重试未同步影片：%v", client.items)
	}
	if err := syncer.step(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.fail = true
	failed := false
	for i := 0; i < 10000; i++ {
		if err := syncer.step(context.Background()); err != nil {
			failed = true
			break
		}
	}
	if !failed {
		var state managedSyncState
		_ = db.Where("name = ?", "gorse_sync_test").First(&state).Error
		t.Fatalf("反馈同步失败未返回错误，状态：%s", state.Value)
	}
	client.fail = false
	if err := syncer.step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(client.feedback) != 1 || client.feedback[0].FeedbackType != "like" || client.feedback[0].UserId != managedUserID(id) {
		t.Fatalf("反馈重试不正确：%v", client.feedback)
	}
	if err := syncer.step(context.Background()); err != nil {
		t.Fatal(err)
	}
	var row managedSyncState
	if err := db.Where("name = ?", "gorse_sync_test").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var progress managedSyncProgress
	if err := json.Unmarshal([]byte(row.Value), &progress); err != nil || !progress.Ready {
		t.Fatalf("同步完成状态错误：%+v %v", progress, err)
	}
}

func TestManagedSyncWithRealGorse(t *testing.T) {
	endpoint, driver := os.Getenv("LIBREFANZA_TEST_GORSE_URL"), os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if endpoint == "" || driver == "" {
		t.Skip("需要隔离的推荐服务和测试数据库")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	id := "SYNC-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	movie := catalog.Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", movie.ID).Delete(&catalog.Movie{})
	feedback := UserFeedback{UserID: id, NormalizedID: id, FeedbackType: "like", Score: 1}
	if err := db.Create(&feedback).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", feedback.ID).Delete(&UserFeedback{})
	name := "gorse_sync_real_test"
	defer db.Where("name = ?", name).Delete(&managedSyncState{})
	recentName := "gorse_sync_recent_test"
	defer db.Where("name = ?", recentName).Delete(&managedSyncState{})
	recentMoviesName := "gorse_sync_recent_movies_test"
	defer db.Where("name = ?", recentMoviesName).Delete(&managedSyncState{})
	client := NewGorseClient(endpoint, "")
	remote := gorse.NewGorseClient(endpoint, "")
	defer remote.DeleteItem(context.Background(), id)
	defer remote.DeleteUser(context.Background(), managedUserID(id))
	syncer := newManagedSync(db, client, name)
	runCycle := func() {
		t.Helper()
		for i := 0; i < 10000; i++ {
			err := syncer.step(context.Background())
			if errors.Is(err, ErrManagedSyncIdle) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("同步没有完成")
	}
	runCycle()
	remoteMovie, err := remote.GetItem(context.Background(), id)
	if err != nil || remoteMovie.ItemId != id {
		t.Fatalf("影片未同步：%v", err)
	}
	recentMovies := newManagedSync(db, client, recentMoviesName)
	runRecentMovies := func() {
		t.Helper()
		for i := 0; i < 10000; i++ {
			err := recentMovies.recentMovieStep(context.Background(), time.Time{})
			if errors.Is(err, ErrManagedSyncIdle) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("增量影片同步没有完成")
	}
	runRecentMovies()
	if err := db.Model(&movie).Update("is_hidden", true).Error; err != nil {
		t.Fatal(err)
	}
	runRecentMovies()
	remoteMovie, err = remote.GetItem(context.Background(), id)
	if err != nil || !remoteMovie.IsHidden {
		t.Fatalf("隐藏状态未同步：%v", err)
	}
	likes, err := remote.ListFeedbacks(context.Background(), "like", managedUserID(id))
	if err != nil || len(likes) != 1 {
		t.Fatalf("喜欢反馈未同步：%d %v", len(likes), err)
	}
	recent := newManagedSync(db, client, recentName)
	runRecent := func() {
		t.Helper()
		for i := 0; i < 10000; i++ {
			err := recent.recentStep(context.Background())
			if errors.Is(err, ErrManagedSyncIdle) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("增量反馈同步没有完成")
	}
	runRecent()
	if err := db.Model(&feedback).Updates(map[string]any{"feedback_type": "dislike", "score": -1}).Error; err != nil {
		t.Fatal(err)
	}
	runRecent()
	likes, err = remote.ListFeedbacks(context.Background(), "like", managedUserID(id))
	if err != nil || len(likes) != 0 {
		t.Fatalf("旧反馈未清理：%d %v", len(likes), err)
	}
	dislikes, err := remote.ListFeedbacks(context.Background(), "dislike", managedUserID(id))
	if err != nil || len(dislikes) != 1 {
		t.Fatalf("新反馈未同步：%d %v", len(dislikes), err)
	}
}
