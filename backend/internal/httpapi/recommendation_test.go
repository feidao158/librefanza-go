package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func TestRecommendationAPIWorkflow(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	cfg := config.Database{
		Driver:   driver,
		Host:     "127.0.0.1",
		Port:     port,
		Name:     "librefanza_test",
		User:     "test",
		Password: "integration-test-only",
	}
	if err := store.Save(config.Config{Installed: true, Database: cfg}); err != nil {
		t.Fatal(err)
	}
	provider := database.NewProvider(store)
	defer provider.Close()

	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(setup.NewService(store, setupBackend{}), provider)

	// 创建测试影片
	idPrefix := fmt.Sprintf("HTTPREC%d", time.Now().UnixNano())
	movieIDs := make([]string, 6)
	for i := 0; i < 6; i++ {
		id := catalog.NormalizeProductID(fmt.Sprintf("%s%d", idPrefix, i))
		movieIDs[i] = id
		m := catalog.Movie{NormalizedID: id, CompressedID: id}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		defer db.Where("normalized_id = ?", id).Delete(&catalog.Movie{})

		item := catalog.SourceItem{
			Source:       "fanza",
			ExternalID:   id,
			NormalizedID: id,
			Title:        "测试影片 " + id,
			MetadataJSON: `{"genres":["剧情","独占"]}`,
		}
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
		defer db.Where("normalized_id = ?", id).Delete(&catalog.SourceItem{})
	}

	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())

	// 1. 获取下一个未标注影片
	req := httptest.NewRequest(http.MethodGet, "/api/v1/annotations/next", nil)
	req.Header.Set("X-Client-ID", clientID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get next annotation: %d %s", rec.Code, rec.Body.String())
	}
	var nextResp struct {
		Data struct {
			Movie *catalog.Movie `json:"movie"`
			Stats struct {
				Total int  `json:"total"`
				Ready bool `json:"ready"`
			} `json:"stats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &nextResp); err != nil || nextResp.Data.Movie == nil {
		t.Fatalf("invalid next annotation response: %v, body: %s", err, rec.Body.String())
	}

	// 2. 连续提交 4 次标注
	for i := 0; i < 4; i++ {
		action := "like"
		if i%2 == 1 {
			action = "dislike"
		}
		body, _ := json.Marshal(map[string]string{
			"normalized_id": movieIDs[i],
			"action":        action,
		})
		postReq := httptest.NewRequest(http.MethodPost, "/api/v1/annotations/feedback", bytes.NewReader(body))
		postReq.Header.Set("X-Client-ID", clientID)
		postReq.Header.Set("Content-Type", "application/json")
		postRec := httptest.NewRecorder()
		router.ServeHTTP(postRec, postReq)
		if postRec.Code != http.StatusOK {
			t.Fatalf("submit feedback %d: %d %s", i, postRec.Code, postRec.Body.String())
		}
	}

	// 3. 此时查看猜你喜欢接口，应返回 ready=false
	recReq := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations", nil)
	recReq.Header.Set("X-Client-ID", clientID)
	recRecorder := httptest.NewRecorder()
	router.ServeHTTP(recRecorder, recReq)
	if recRecorder.Code != http.StatusOK {
		t.Fatalf("get recommendations: %d %s", recRecorder.Code, recRecorder.Body.String())
	}
	var recsResp struct {
		Data struct {
			Ready  bool            `json:"ready"`
			Movies []catalog.Movie `json:"movies"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recRecorder.Body.Bytes(), &recsResp); err != nil {
		t.Fatal(err)
	}
	if recsResp.Data.Ready || len(recsResp.Data.Movies) != 0 {
		t.Fatalf("recommendations should not be ready before threshold: %+v", recsResp.Data)
	}

	// 4. 提交第 5 次标注，达到 5 次阈值
	candidatesReq := httptest.NewRequest(http.MethodGet, "/api/v1/annotations/candidates?exclude="+movieIDs[4], nil)
	candidatesReq.Header.Set("X-Client-ID", clientID)
	candidatesResponse := httptest.NewRecorder()
	router.ServeHTTP(candidatesResponse, candidatesReq)
	if candidatesResponse.Code != http.StatusOK || strings.Contains(candidatesResponse.Body.String(), movieIDs[4]) {
		t.Fatalf("候选排除错误: %d %s", candidatesResponse.Code, candidatesResponse.Body.String())
	}
	body5, _ := json.Marshal(map[string]any{
		"normalized_id": movieIDs[4],
		"action":        "uncertain",
		"prefetched":    true,
		"candidate_ids": []string{movieIDs[0], movieIDs[4], movieIDs[5]},
	})
	postReq5 := httptest.NewRequest(http.MethodPost, "/api/v1/annotations/feedback", bytes.NewReader(body5))
	postReq5.Header.Set("X-Client-ID", clientID)
	postReq5.Header.Set("Content-Type", "application/json")
	postRec5 := httptest.NewRecorder()
	router.ServeHTTP(postRec5, postReq5)
	if postRec5.Code != http.StatusOK {
		t.Fatalf("submit feedback 5: %d %s", postRec5.Code, postRec5.Body.String())
	}
	var prefetchResult struct {
		Data struct {
			NextMovie   *catalog.Movie `json:"next_movie"`
			EligibleIDs []string       `json:"eligible_ids"`
		} `json:"data"`
	}
	if json.Unmarshal(postRec5.Body.Bytes(), &prefetchResult) != nil || prefetchResult.Data.NextMovie != nil || len(prefetchResult.Data.EligibleIDs) != 1 || prefetchResult.Data.EligibleIDs[0] != movieIDs[5] {
		t.Fatalf("反馈候选校验错误: %s", postRec5.Body.String())
	}

	// 5. 未配置推荐服务时明确报告不可用。
	recReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations", nil)
	recReq2.Header.Set("X-Client-ID", clientID)
	recRecorder2 := httptest.NewRecorder()
	router.ServeHTTP(recRecorder2, recReq2)
	if recRecorder2.Code != http.StatusServiceUnavailable || !strings.Contains(recRecorder2.Body.String(), "recommendation_unavailable") {
		t.Fatalf("get recommendations 2: %d %s", recRecorder2.Code, recRecorder2.Body.String())
	}

	likesReq := httptest.NewRequest(http.MethodGet, "/api/v1/annotations/likes?page=1&page_size=1", nil)
	likesReq.Header.Set("X-Client-ID", clientID)
	likesRec := httptest.NewRecorder()
	router.ServeHTTP(likesRec, likesReq)
	var likes struct {
		Data []catalog.Movie `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if likesRec.Code != http.StatusOK || json.Unmarshal(likesRec.Body.Bytes(), &likes) != nil || likes.Meta.Total != 2 || len(likes.Data) != 1 || likes.Data[0].NormalizedID != movieIDs[2] {
		t.Fatalf("喜欢列表及分页不正确: %d %s", likesRec.Code, likesRec.Body.String())
	}
	otherReq := httptest.NewRequest(http.MethodGet, "/api/v1/annotations/likes", nil)
	otherReq.Header.Set("X-Client-ID", clientID+"-other")
	otherRec := httptest.NewRecorder()
	router.ServeHTTP(otherRec, otherReq)
	if otherRec.Code != http.StatusOK || strings.Contains(otherRec.Body.String(), movieIDs[0]) {
		t.Fatalf("喜欢列表不得泄漏其他用户数据: %d %s", otherRec.Code, otherRec.Body.String())
	}

	// 6. 在猜你喜欢页面提交交互反馈
	recFeedbackBody, _ := json.Marshal(map[string]string{
		"normalized_id": movieIDs[5],
		"action":        "like",
	})
	recFbReq := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/feedback", bytes.NewReader(recFeedbackBody))
	recFbReq.Header.Set("X-Client-ID", clientID)
	recFbReq.Header.Set("Content-Type", "application/json")
	recFbRec := httptest.NewRecorder()
	router.ServeHTTP(recFbRec, recFbReq)
	if recFbRec.Code != http.StatusOK {
		t.Fatalf("submit rec feedback: %d %s", recFbRec.Code, recFbRec.Body.String())
	}
	var entry logbook.Entry
	if err := db.Where("source = ? AND event = ?", "recommendation", "recommendation_feedback").Order("id DESC").First(&entry).Error; err != nil {
		t.Fatalf("反馈日志未保存：%v", err)
	}
	if !strings.Contains(entry.DetailsJSON, `"origin":"recommendation"`) || !strings.Contains(entry.DetailsJSON, `"action":"like"`) || strings.Contains(entry.DetailsJSON, clientID) || strings.Contains(entry.DetailsJSON, movieIDs[5]) {
		t.Fatalf("反馈日志内容不正确：%s", entry.DetailsJSON)
	}
}
