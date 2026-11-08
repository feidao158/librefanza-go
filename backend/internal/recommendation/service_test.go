package recommendation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/settings"
)

type mockGorseClient struct {
	insertedUsers     []gorse.User
	insertedItems     []gorse.Item
	insertedFeedbacks []gorse.Feedback
	recommendResults  []string
	recommendErr      error
	recommendCalls    int
	recommendUser     string
}

func (m *mockGorseClient) InsertUser(ctx context.Context, user gorse.User) (gorse.RowAffected, error) {
	m.insertedUsers = append(m.insertedUsers, user)
	return gorse.RowAffected{RowAffected: 1}, nil
}

func (m *mockGorseClient) InsertItem(ctx context.Context, item gorse.Item) (gorse.RowAffected, error) {
	m.insertedItems = append(m.insertedItems, item)
	return gorse.RowAffected{RowAffected: 1}, nil
}

func (m *mockGorseClient) InsertFeedback(ctx context.Context, feedbacks []gorse.Feedback) (gorse.RowAffected, error) {
	m.insertedFeedbacks = append(m.insertedFeedbacks, feedbacks...)
	return gorse.RowAffected{RowAffected: len(feedbacks)}, nil
}

func (m *mockGorseClient) DeleteFeedbacks(ctx context.Context, userID, itemID string) (gorse.RowAffected, error) {
	return gorse.RowAffected{}, nil
}

func (m *mockGorseClient) GetRecommend(ctx context.Context, userId string, category string, n, offset int) ([]string, error) {
	m.recommendCalls++
	m.recommendUser = userId
	if m.recommendErr != nil {
		return nil, m.recommendErr
	}
	return m.recommendResults, nil
}

func TestMockGorseClient(t *testing.T) {
	client := &mockGorseClient{}
	ctx := context.Background()
	if _, err := client.InsertUser(ctx, gorse.User{UserId: "u1"}); err != nil {
		t.Fatal(err)
	}
	if len(client.insertedUsers) != 1 || client.insertedUsers[0].UserId != "u1" {
		t.Fatalf("unexpected users: %v", client.insertedUsers)
	}
	if _, err := client.InsertItem(ctx, gorse.Item{ItemId: "m1"}); err != nil {
		t.Fatal(err)
	}
	if len(client.insertedItems) != 1 || client.insertedItems[0].ItemId != "m1" {
		t.Fatalf("unexpected items: %v", client.insertedItems)
	}
	if _, err := client.InsertFeedback(ctx, []gorse.Feedback{{FeedbackType: "like", UserId: "u1", ItemId: "m1"}}); err != nil {
		t.Fatal(err)
	}
	if len(client.insertedFeedbacks) != 1 {
		t.Fatalf("unexpected feedbacks count: %d", len(client.insertedFeedbacks))
	}
	client.recommendResults = []string{"m1", "m2"}
	recs, err := client.GetRecommend(ctx, "u1", "", 10, 0)
	if err != nil || len(recs) != 2 {
		t.Fatalf("unexpected recommend: %v, err=%v", recs, err)
	}
}

func TestRecommendationAndAnnotation(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{
		Driver:   driver,
		Host:     "127.0.0.1",
		Port:     port,
		Name:     "librefanza_test",
		User:     "test",
		Password: "integration-test-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}

	mockGorse := &mockGorseClient{}
	svc := NewService(db, func(ctx context.Context) (settings.GorseSettings, error) {
		return settings.GorseSettings{
			EntryPoint:       "http://127.0.0.1:8088",
			Enabled:          true,
			MinFeedbackCount: 5,
		}, nil
	}, mockGorse)

	userID := fmt.Sprintf("test-user-%d", time.Now().UnixNano())
	defer db.Where("user_id = ?", userID).Delete(&UserFeedback{})
	defer db.Where("user_id = ?", userID).Delete(&UserRecommendationProfile{})

	// 1. 验证初始状态未就绪
	stats, err := svc.GetStats(context.Background(), userID)
	if err != nil {
		t.Fatalf("get stats error: %v", err)
	}
	if stats.Ready || stats.Total != 0 || stats.MinRequired != 5 {
		t.Fatalf("unexpected initial stats: %+v", stats)
	}

	// 插入测试影片数据
	actressLiked := "测试演员A"
	actressDisliked := "测试演员B"
	genreLiked := "单体作品"
	genreDisliked := "恐怖"

	movieIDs := make([]string, 7)
	for i := 0; i < 7; i++ {
		id := catalog.NormalizeProductID(fmt.Sprintf("TESTREC%d%d", time.Now().UnixNano(), i))
		movieIDs[i] = id
		m := catalog.Movie{NormalizedID: id, CompressedID: id}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		defer db.Where("normalized_id = ?", id).Delete(&catalog.Movie{})

		var meta string
		if i == 0 || i == 1 {
			// 喜欢候选
			meta = fmt.Sprintf(`{"cast":[{"name":%q}],"genres":[%q],"makers":["S1"]}`, actressLiked, genreLiked)
		} else if i == 2 {
			// 不喜欢候选
			meta = fmt.Sprintf(`{"cast":[{"name":%q}],"genres":[%q],"makers":["OTHER"]}`, actressDisliked, genreDisliked)
		} else {
			meta = `{"cast":[{"name":"一般演员"}],"genres":["剧情"]}`
		}

		item := catalog.SourceItem{
			Source:        "fanza",
			ExternalID:    id,
			NormalizedID:  id,
			Title:         "测试影片 " + id,
			MetadataJSON:  meta,
			CoverImageURL: "https://example.com/cover.jpg",
		}
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
		defer db.Where("normalized_id = ?", id).Delete(&catalog.SourceItem{})
	}

	// 2. 连续提交 4 次标注，验证阈值未达到
	for i := 0; i < 4; i++ {
		action := "like"
		if i == 2 {
			action = "dislike"
		} else if i == 3 {
			action = "uncertain"
		}
		s, _, err := svc.SaveFeedback(context.Background(), userID, FeedbackInput{
			NormalizedID: movieIDs[i],
			Action:       action,
		})
		if err != nil {
			t.Fatalf("save feedback %d error: %v", i, err)
		}
		if s.Total != i+1 {
			t.Fatalf("expected total %d, got %d", i+1, s.Total)
		}
		if s.Ready {
			t.Fatalf("should not be ready at count %d", s.Total)
		}
	}

	// 此时未达到阈值，调用猜你喜欢应返回 ready=false 和空列表
	recs, recStats, _, err := svc.GetRecommendations(context.Background(), userID, 1, 10)
	if err != nil {
		t.Fatalf("get recommendations error: %v", err)
	}
	if recStats.Ready || len(recs) != 0 {
		t.Fatalf("recommendations should not be active yet")
	}

	// 3. 提交第 5 次标注，达到生效阈值
	stats5, _, err := svc.SaveFeedback(context.Background(), userID, FeedbackInput{
		NormalizedID: movieIDs[4],
		Action:       "like",
	})
	if err != nil {
		t.Fatalf("save feedback 5 error: %v", err)
	}
	if !stats5.Ready || stats5.Total != 5 {
		t.Fatalf("expected ready=true at 5 feedbacks, got: %+v", stats5)
	}

	// 验证 profile 中的偏好参数已更新
	var profile UserRecommendationProfile
	if err := db.Where("user_id = ?", userID).First(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var params PreferenceParameters
	if err := json.Unmarshal([]byte(profile.ParametersJSON), &params); err != nil {
		t.Fatal(err)
	}
	if params.Actresses[actressLiked] <= 0 {
		t.Fatalf("expected positive weight for liked actress, got %f", params.Actresses[actressLiked])
	}
	if params.Actresses[actressDisliked] >= 0 {
		t.Fatalf("expected negative weight for disliked actress, got %f", params.Actresses[actressDisliked])
	}
	if params.Actresses["一般演员"] != 1 {
		t.Fatalf("uncertain feedback must not increase preference: %f", params.Actresses["一般演员"])
	}
	duplicate := catalog.SourceItem{
		Source: "mgstage", ExternalID: movieIDs[6], NormalizedID: movieIDs[6],
		MetadataJSON: `{"genres":["剧情"]}`,
	}
	if err := db.Create(&duplicate).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", duplicate.ID).Delete(&catalog.SourceItem{})

	// 4. 测试猜你喜欢生效（Fallback 参数打分推荐）
	recs, recStats, total, err := svc.GetRecommendations(context.Background(), userID, 1, 10)
	if err != nil {
		t.Fatalf("get recommendations error: %v", err)
	}
	if !recStats.Ready {
		t.Fatalf("expected recStats.Ready=true")
	}
	var recLog logbook.Entry
	if err := db.Where("source = ? AND event = ?", "recommendation", "recommendation_served").Order("id DESC").First(&recLog).Error; err != nil || !strings.Contains(recLog.DetailsJSON, `"algorithm":"local"`) {
		t.Fatalf("本地推荐日志未落库：%+v %v", recLog, err)
	}
	if total == 0 || len(recs) == 0 {
		t.Fatalf("expected recommendations to return items, got %d", len(recs))
	}
	if len(recs) != 2 || recs[0].NormalizedID != movieIDs[5] {
		t.Fatalf("duplicate source metadata must not change ranking: %+v", recs)
	}
	var oldMovie catalog.Movie
	if err := db.Where("normalized_id = ?", movieIDs[6]).First(&oldMovie).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO movie_cast (movie_id, name) VALUES (?, ?)", oldMovie.ID, actressLiked).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM movie_cast WHERE movie_id = ?", oldMovie.ID)
	now := time.Now().UTC()
	var recent []catalog.Movie
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("RECENT-%s-%d", userID, i)
		recent = append(recent, catalog.Movie{NormalizedID: id, CompressedID: id, ReleaseDate: &now})
	}
	if err := db.CreateInBatches(recent, 100).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("normalized_id LIKE ?", "RECENT-"+userID+"-%").Delete(&catalog.Movie{})
	recs, _, _, err = svc.GetRecommendations(context.Background(), userID, 1, 10)
	if err != nil || len(recs) == 0 || recs[0].NormalizedID != movieIDs[6] {
		t.Fatalf("preferred actress must recall older movies: %v %v", recs, err)
	}
	if err := db.Where("normalized_id LIKE ?", "RECENT-"+userID+"-%").Delete(&catalog.Movie{}).Error; err != nil {
		t.Fatal(err)
	}

	// 推荐服务结果不足时使用本地候选补足，翻页不能重复。
	mockGorse.recommendResults = []string{movieIDs[5]}
	firstPage, _, totalMixed, err := svc.GetRecommendations(context.Background(), userID, 1, 1)
	if err != nil || len(firstPage) != 1 || firstPage[0].NormalizedID != movieIDs[5] || totalMixed < 2 {
		t.Fatalf("推荐第一页及候选总数不正确：%v %d %v", firstPage, totalMixed, err)
	}
	secondPage, _, _, err := svc.GetRecommendations(context.Background(), userID, 2, 1)
	if err != nil || len(secondPage) != 1 || secondPage[0].NormalizedID != movieIDs[6] {
		t.Fatalf("推荐第二页应由本地候选补足且不能重复：%v %v", secondPage, err)
	}
	var dislikedCandidate catalog.Movie
	if err := db.Where("normalized_id = ?", movieIDs[5]).First(&dislikedCandidate).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO movie_cast (movie_id, name) VALUES (?, ?)", dislikedCandidate.ID, actressDisliked).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM movie_cast WHERE movie_id = ?", dislikedCandidate.ID)
	mockGorse.recommendResults = []string{movieIDs[5], movieIDs[6]}
	reordered, _, _, err := svc.GetRecommendations(context.Background(), userID, 1, 2)
	if err != nil || len(reordered) != 2 || reordered[0].NormalizedID != movieIDs[6] {
		t.Fatalf("推荐排序应考虑已保存的喜欢和不喜欢特征：%v %v", reordered, err)
	}

	// 5. 测试 Gorse 推荐协同过滤返回
	mockGorse.recommendResults = []string{movieIDs[0], movieIDs[3], movieIDs[5], movieIDs[6]}
	gorseRecs, _, _, err := svc.GetRecommendations(context.Background(), userID, 1, 10)
	if err != nil {
		t.Fatalf("gorse recommendations error: %v", err)
	}
	if len(gorseRecs) != 2 || gorseRecs[0].NormalizedID != movieIDs[6] {
		t.Fatalf("expected gorse recommendation first item to be %s", movieIDs[6])
	}

	// 6. 测试在猜你喜欢页面继续标注
	stats6, _, err := svc.SaveFeedback(context.Background(), userID, FeedbackInput{
		NormalizedID: movieIDs[5],
		Action:       "like",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats6.Total != 6 || stats6.Liked < 3 {
		t.Fatalf("unexpected stats after 6th rating: %+v", stats6)
	}

	var extra []UserFeedback
	for i := 0; i < 1001; i++ {
		extra = append(extra, UserFeedback{UserID: userID, NormalizedID: fmt.Sprintf("EXTRA-%d", i), FeedbackType: "uncertain"})
	}
	if err := db.CreateInBatches(extra, 100).Error; err != nil {
		t.Fatal(err)
	}
	mockGorse.recommendResults = nil
	recs, _, _, err = svc.GetRecommendations(context.Background(), userID, 1, 10)
	if err != nil || len(recs) != 1 || recs[0].NormalizedID != movieIDs[6] {
		t.Fatalf("rated movies must remain excluded with many feedbacks: %v %v", recs, err)
	}
	next, _, err := svc.GetNextAnnotation(context.Background(), userID, movieIDs[6])
	if err != nil || next != nil {
		t.Fatalf("annotation exclusions must apply with many feedbacks: %v %v", next, err)
	}

	t.Setenv("LIBREFANZA_MANAGED_GORSE", "true")
	defer db.Where("name = ?", managedSyncName).Delete(&managedSyncState{})
	mockGorse.recommendResults = []string{movieIDs[6]}
	calls := mockGorse.recommendCalls
	if _, _, _, err := svc.GetRecommendations(context.Background(), userID, 1, 10); err != nil || mockGorse.recommendCalls != calls {
		t.Fatalf("首次同步完成前不应使用 Gorse：%v", err)
	}
	if err := db.Create(&managedSyncState{Name: managedSyncName, Value: `{"phase":"movies","ready":true}`, UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.GetRecommendations(context.Background(), userID, 1, 10); err != nil || mockGorse.recommendCalls != calls+1 || mockGorse.recommendUser != managedUserID(userID) {
		t.Fatalf("首次同步完成后应使用内部用户标识：%v", err)
	}
	defer db.Where("name = ?", managedSyncHealthName).Delete(&managedSyncState{})
	if err := SetManagedSyncHealthy(context.Background(), db, false); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.GetRecommendations(context.Background(), userID, 1, 10); err != nil || mockGorse.recommendCalls != calls+1 {
		t.Fatalf("同步异常时应回退本地推荐：%v", err)
	}
}
