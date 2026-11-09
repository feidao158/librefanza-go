package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"gorm.io/gorm"
)

var (
	ErrInvalidAction    = errors.New("invalid feedback action")
	ErrMovieNotFound    = errors.New("movie not found")
	ErrGorseUnavailable = errors.New("recommendation service unavailable")
)

// Service 推荐与人工标注服务
type Service struct {
	db          *gorm.DB
	settingsFn  func(context.Context) (settings.GorseSettings, error)
	catalogRepo *catalog.Repository
	gorseClient GorseClientInterface
}

// NewService 创建推荐与人工标注服务实例
func NewService(db *gorm.DB, settingsFn func(context.Context) (settings.GorseSettings, error), clients ...GorseClientInterface) *Service {
	svc := &Service{
		db:          db,
		settingsFn:  settingsFn,
		catalogRepo: catalog.NewRepository(db),
	}
	if len(clients) > 0 && clients[0] != nil {
		svc.gorseClient = clients[0]
	}
	return svc
}

// getGorseClient 获取或初始化 Gorse 客户端
func (s *Service) getGorseClient(ctx context.Context) (GorseClientInterface, settings.GorseSettings, bool) {
	if s.settingsFn == nil {
		return s.gorseClient, settings.GorseSettings{MinFeedbackCount: 5}, s.gorseClient != nil
	}
	cfg, err := s.settingsFn(ctx)
	if err != nil {
		return s.gorseClient, settings.GorseSettings{MinFeedbackCount: 5}, s.gorseClient != nil
	}
	if cfg.MinFeedbackCount <= 0 {
		cfg.MinFeedbackCount = 5
	}
	if s.gorseClient != nil {
		return s.gorseClient, cfg, cfg.Enabled
	}
	if cfg.Enabled && cfg.EntryPoint != "" {
		return NewGorseClient(cfg.EntryPoint, cfg.APIKey), cfg, true
	}
	return nil, cfg, false
}

// GetStats 获取用户当前的标注与推荐统计
func (s *Service) GetStats(ctx context.Context, userID string) (Stats, error) {
	_, cfg, _ := s.getGorseClient(ctx)
	minRequired := cfg.MinFeedbackCount
	if minRequired <= 0 {
		minRequired = 5
	}

	var profile UserRecommendationProfile
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Stats{
			Total:       0,
			Liked:       0,
			Disliked:    0,
			Uncertain:   0,
			MinRequired: minRequired,
			Ready:       false,
		}, nil
	}
	if err != nil {
		return Stats{}, err
	}

	return Stats{
		Total:       profile.FeedbackCount,
		Liked:       profile.LikeCount,
		Disliked:    profile.DislikeCount,
		Uncertain:   profile.UncertainCount,
		MinRequired: minRequired,
		Ready:       profile.FeedbackCount >= minRequired,
	}, nil
}

// GetLikedMovies 按最近标注时间读取当前用户喜欢的可见作品。
func (s *Service) GetLikedMovies(ctx context.Context, userID string, page, pageSize int) ([]catalog.Movie, int64, error) {
	visible := s.db.WithContext(ctx).Model(&catalog.Movie{}).Select("normalized_id").Where("is_hidden = ?", false)
	query := s.db.WithContext(ctx).Model(&UserFeedback{}).Where("user_id = ? AND feedback_type = ?", userID, "like").Where("normalized_id IN (?)", visible)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var feedbacks []UserFeedback
	if err := query.Order("updated_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&feedbacks).Error; err != nil {
		return nil, 0, err
	}
	if len(feedbacks) == 0 {
		return []catalog.Movie{}, total, nil
	}
	ids := make([]string, 0, len(feedbacks))
	order := make(map[string]int, len(feedbacks))
	for i, feedback := range feedbacks {
		ids = append(ids, feedback.NormalizedID)
		order[feedback.NormalizedID] = i
	}
	var movies []catalog.Movie
	if err := s.db.WithContext(ctx).Where("normalized_id IN ? AND is_hidden = ?", ids, false).Find(&movies).Error; err != nil {
		return nil, 0, err
	}
	sort.Slice(movies, func(i, j int) bool { return order[movies[i].NormalizedID] < order[movies[j].NormalizedID] })
	if err := s.catalogRepo.Populate(ctx, movies, false); err != nil {
		return nil, 0, err
	}
	return movies, total, nil
}

// GetNextAnnotation 随机拉取一部未标注的番号供用户人工标注
func (s *Service) GetNextAnnotation(ctx context.Context, userID string, excludeID string) (*catalog.Movie, Stats, error) {
	stats, err := s.GetStats(ctx, userID)
	if err != nil {
		return nil, stats, err
	}

	// 使用子查询排除全部已标注影片，避免参数数量限制。
	ratedQuery := s.db.WithContext(ctx).Model(&UserFeedback{}).Select("normalized_id").Where("user_id = ?", userID)
	db := s.db.WithContext(ctx).Model(&catalog.Movie{}).Where("is_hidden = ?", false).Where("normalized_id NOT IN (?)", ratedQuery)
	if excludeID != "" {
		db = db.Where("normalized_id <> ?", excludeID)
	}

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return nil, stats, err
	}
	if count == 0 {
		return nil, stats, nil
	}

	// 随机选取 offset
	offset := 0
	if count > 1 {
		offset = rand.Intn(int(count))
	}

	var candidates []catalog.Movie
	if err := db.Offset(offset).Limit(5).Find(&candidates).Error; err != nil {
		return nil, stats, err
	}
	if len(candidates) == 0 {
		// 回退从第一页获取
		if err := db.Offset(0).Limit(5).Find(&candidates).Error; err != nil {
			return nil, stats, err
		}
	}
	if len(candidates) == 0 {
		return nil, stats, nil
	}

	// 组装影片封面与来源版本数据
	movies := []catalog.Movie{candidates[0]}
	if err := s.catalogRepo.Populate(ctx, movies, false); err != nil {
		return nil, stats, err
	}

	return &movies[0], stats, nil
}

// movieFeatures 影片提取的推荐特征
type movieFeatures struct {
	Actresses []string
	Genres    []string
	Makers    []string
	Labels    []string
	Directors []string
	Title     string
	Date      *time.Time
}

// extractFeatures 从影片中提取演员、分类、厂牌等参数
func (s *Service) extractFeatures(ctx context.Context, movie catalog.Movie) (movieFeatures, error) {
	feat := movieFeatures{
		Date: movie.ReleaseDate,
	}

	// 查询演员表
	type castRow struct {
		Name string
	}
	var castRows []castRow
	if err := s.db.WithContext(ctx).Table("movie_cast").Select("name").Where("movie_id = ?", movie.ID).Find(&castRows).Error; err != nil {
		return feat, err
	}
	for _, r := range castRows {
		name := strings.TrimSpace(r.Name)
		if name != "" {
			feat.Actresses = append(feat.Actresses, name)
		}
	}

	// 从首选来源项与所有版本提取 metadata
	var items []catalog.SourceItem
	if movie.PreferredItem != nil {
		items = append(items, *movie.PreferredItem)
	}
	items = append(items, movie.Versions...)

	if len(items) == 0 {
		if err := s.db.WithContext(ctx).Where("normalized_id = ?", movie.NormalizedID).Find(&items).Error; err != nil {
			return feat, err
		}
	}

	seenActresses := make(map[string]bool)
	for _, a := range feat.Actresses {
		seenActresses[a] = true
	}
	seenGenres := make(map[string]bool)
	seenMakers := make(map[string]bool)
	seenLabels := make(map[string]bool)
	seenDirectors := make(map[string]bool)

	for _, item := range items {
		if feat.Title == "" && item.Title != "" {
			feat.Title = item.Title
		}
		if item.MetadataJSON == "" {
			continue
		}
		var meta struct {
			Cast []struct {
				Name string `json:"name"`
			} `json:"cast"`
			Genres    []string `json:"genres"`
			Labels    []string `json:"labels"`
			Makers    []string `json:"makers"`
			Directors []string `json:"directors"`
		}
		if json.Unmarshal([]byte(item.MetadataJSON), &meta) != nil {
			continue
		}
		for _, c := range meta.Cast {
			name := strings.TrimSpace(c.Name)
			if name != "" && !seenActresses[name] {
				seenActresses[name] = true
				feat.Actresses = append(feat.Actresses, name)
			}
		}
		for _, g := range meta.Genres {
			g = strings.TrimSpace(g)
			if g != "" && !seenGenres[g] {
				seenGenres[g] = true
				feat.Genres = append(feat.Genres, g)
			}
		}
		for _, m := range meta.Makers {
			m = strings.TrimSpace(m)
			if m != "" && !seenMakers[m] {
				seenMakers[m] = true
				feat.Makers = append(feat.Makers, m)
			}
		}
		for _, l := range meta.Labels {
			l = strings.TrimSpace(l)
			if l != "" && !seenLabels[l] {
				seenLabels[l] = true
				feat.Labels = append(feat.Labels, l)
			}
		}
		for _, d := range meta.Directors {
			d = strings.TrimSpace(d)
			if d != "" && !seenDirectors[d] {
				seenDirectors[d] = true
				feat.Directors = append(feat.Directors, d)
			}
		}
	}

	return feat, nil
}

// SaveFeedback 保存用户对番号的人工标注或猜你喜欢交互反馈，更新参数模型并推送 Gorse
func (s *Service) SaveFeedback(ctx context.Context, userID string, input FeedbackInput) (Stats, *catalog.Movie, error) {
	action := strings.ToLower(strings.TrimSpace(input.Action))
	if action != "like" && action != "dislike" && action != "uncertain" {
		return Stats{}, nil, ErrInvalidAction
	}
	id := strings.ToUpper(strings.TrimSpace(input.NormalizedID))
	if catalog.IsProductID(id) {
		id = catalog.NormalizeProductID(id)
	}

	// 查询影片信息
	var movie catalog.Movie
	if err := s.db.WithContext(ctx).Where("normalized_id = ?", id).First(&movie).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Stats{}, nil, ErrMovieNotFound
		}
		return Stats{}, nil, err
	}

	features, err := s.extractFeatures(ctx, movie)
	if err != nil {
		return Stats{}, nil, err
	}

	score := 0.0
	switch action {
	case "like":
		score = 1.0
	case "dislike":
		score = -1.0
	case "uncertain":
		score = 0
	}

	now := time.Now().UTC()

	// 事务内保存用户反馈并更新画像特征参数
	var updatedStats Stats
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var oldFeedback UserFeedback
		hasOld := false
		err := tx.Where("user_id = ? AND normalized_id = ?", userID, id).First(&oldFeedback).Error
		if err == nil {
			hasOld = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// 保存或更新 feedback 记录
		feedback := UserFeedback{
			UserID:       userID,
			NormalizedID: id,
			FeedbackType: action,
			Score:        score,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if hasOld {
			feedback.ID = oldFeedback.ID
			feedback.CreatedAt = oldFeedback.CreatedAt
			if err := tx.Save(&feedback).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Create(&feedback).Error; err != nil {
				return err
			}
		}

		// 加载或初始化 Profile
		var profile UserRecommendationProfile
		err = tx.Where("user_id = ?", userID).First(&profile).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			profile = UserRecommendationProfile{
				UserID:         userID,
				FeedbackCount:  0,
				LikeCount:      0,
				DislikeCount:   0,
				UncertainCount: 0,
				ParametersJSON: "{}",
				CreatedAt:      now,
				UpdatedAt:      now,
			}
		} else if err != nil {
			return err
		}

		// 解析现有偏好参数
		params := PreferenceParameters{
			Actresses: make(map[string]float64),
			Genres:    make(map[string]float64),
			Makers:    make(map[string]float64),
			Labels:    make(map[string]float64),
			Directors: make(map[string]float64),
		}
		if profile.ParametersJSON != "" && profile.ParametersJSON != "{}" {
			_ = json.Unmarshal([]byte(profile.ParametersJSON), &params)
		}
		if params.Actresses == nil {
			params.Actresses = make(map[string]float64)
		}
		if params.Genres == nil {
			params.Genres = make(map[string]float64)
		}
		if params.Makers == nil {
			params.Makers = make(map[string]float64)
		}
		if params.Labels == nil {
			params.Labels = make(map[string]float64)
		}
		if params.Directors == nil {
			params.Directors = make(map[string]float64)
		}

		// 计算权重差量 delta
		var delta float64
		if hasOld {
			delta = score - oldFeedback.Score
			switch oldFeedback.FeedbackType {
			case "like":
				profile.LikeCount--
			case "dislike":
				profile.DislikeCount--
			case "uncertain":
				profile.UncertainCount--
			}
		} else {
			delta = score
			profile.FeedbackCount++
		}

		switch action {
		case "like":
			profile.LikeCount++
		case "dislike":
			profile.DislikeCount++
		case "uncertain":
			profile.UncertainCount++
		}

		// 更新细粒度特征参数权重
		for _, a := range features.Actresses {
			params.Actresses[a] += delta
		}
		for _, g := range features.Genres {
			params.Genres[g] += delta
		}
		for _, m := range features.Makers {
			params.Makers[m] += delta
		}
		for _, l := range features.Labels {
			params.Labels[l] += delta
		}
		for _, d := range features.Directors {
			params.Directors[d] += delta
		}
		params.UpdatedAt = now

		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		profile.ParametersJSON = string(data)
		profile.UpdatedAt = now

		if profile.ID == 0 {
			if err := tx.Create(&profile).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Save(&profile).Error; err != nil {
				return err
			}
		}

		_, cfg, _ := s.getGorseClient(ctx)
		minReq := cfg.MinFeedbackCount
		if minReq <= 0 {
			minReq = 5
		}
		updatedStats = Stats{
			Total:       profile.FeedbackCount,
			Liked:       profile.LikeCount,
			Disliked:    profile.DislikeCount,
			Uncertain:   profile.UncertainCount,
			MinRequired: minReq,
			Ready:       profile.FeedbackCount >= minReq,
		}

		return nil
	})
	if err != nil {
		return Stats{}, nil, err
	}

	// 异步或超时同步至 Gorse
	if os.Getenv("LIBREFANZA_MANAGED_GORSE") != "true" {
		go s.syncToGorse(context.Background(), userID, movie, features, action, score, now)
	}

	// 获取下一部推荐标注候选影片
	nextMovie, _, _ := s.GetNextAnnotation(ctx, userID, id)

	return updatedStats, nextMovie, nil
}

// syncToGorse 向 Gorse 推送 Item 与 Feedback 数据
func (s *Service) syncToGorse(ctx context.Context, userID string, movie catalog.Movie, features movieFeatures, action string, score float64, timestamp time.Time) {
	client, _, enabled := s.getGorseClient(ctx)
	if !enabled || client == nil {
		return
	}

	syncCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 准备标签
	var labels []string
	for _, a := range features.Actresses {
		labels = append(labels, "actress:"+a)
	}
	for _, g := range features.Genres {
		labels = append(labels, "genre:"+g)
	}
	for _, m := range features.Makers {
		labels = append(labels, "maker:"+m)
	}

	var categories []string
	if len(features.Genres) > 0 {
		categories = features.Genres
	} else {
		categories = []string{"movie"}
	}

	item := gorse.Item{
		ItemId:     movie.NormalizedID,
		IsHidden:   movie.IsHidden,
		Categories: categories,
		Labels:     labels,
		Timestamp:  timestamp,
		Comment:    features.Title,
	}
	_, _ = client.InsertItem(syncCtx, item)

	// Gorse 常用 feedback 类型: like, read 等
	gorseFeedbackType := action
	if action == "uncertain" {
		gorseFeedbackType = "read"
	}
	feedback := gorse.Feedback{
		FeedbackType: gorseFeedbackType,
		UserId:       userID,
		ItemId:       movie.NormalizedID,
		Value:        score,
		Timestamp:    timestamp,
	}
	_, _ = client.InsertFeedback(syncCtx, []gorse.Feedback{feedback})
}

// GetRecommendations 从 Gorse 获取推荐并保留其排序。
func (s *Service) GetRecommendations(ctx context.Context, userID string, page, pageSize int) (result []catalog.Movie, resultStats Stats, total int64, resultErr error) {
	started := time.Now()
	details := logbook.RecommendationDetails{Gorse: "disabled"}
	defer func() {
		details.Page, details.PageSize = page, pageSize
		details.Returned, details.DurationMS = len(result), time.Since(started).Milliseconds()
		details.Liked, details.Disliked, details.Uncertain = resultStats.Liked, resultStats.Disliked, resultStats.Uncertain
		event, level := "recommendation_served", "info"
		if resultErr != nil {
			event, level = "recommendation_failed", "error"
		}
		logbook.New(s.db, os.Stdout).Record(ctx, logbook.RecommendationEntry(event, level, details))
	}()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 24
	}

	stats, err := s.GetStats(ctx, userID)
	if err != nil {
		return nil, stats, 0, err
	}
	// 人工标注数据不足时，猜你喜欢功能暂未生效
	if !stats.Ready {
		details.Algorithm = "not_ready"
		return []catalog.Movie{}, stats, 0, nil
	}

	// 查询用户所有已标注的番号
	var ratedFeedbacks []UserFeedback
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&ratedFeedbacks).Error; err != nil {
		return nil, stats, 0, err
	}
	ratedMap := make(map[string]string, len(ratedFeedbacks))
	for _, f := range ratedFeedbacks {
		ratedMap[f.NormalizedID] = f.FeedbackType
	}

	// 仅使用推荐服务产生的结果。
	client, _, enabled := s.getGorseClient(ctx)
	var gorseMovies []catalog.Movie
	gorseUserID := userID
	if enabled && os.Getenv("LIBREFANZA_MANAGED_GORSE") == "true" {
		progress, err := ManagedSyncStatus(ctx, s.db)
		if err != nil || !progress.Ready {
			details.Gorse = "syncing"
			return nil, stats, 0, ErrGorseUnavailable
		} else {
			healthy, healthErr := ManagedSyncHealthy(ctx, s.db)
			if healthErr != nil || !healthy {
				details.Gorse = "error"
				return nil, stats, 0, ErrGorseUnavailable
			} else {
				gorseUserID = managedUserID(userID)
			}
		}
	}
	if !enabled || client == nil {
		return nil, stats, 0, ErrGorseUnavailable
	}
	if enabled && client != nil {
		gorseCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		recommendIDs, err := client.GetRecommend(gorseCtx, gorseUserID, "", 100, 0)
		cancel()
		details.Gorse = "empty"
		if err != nil {
			details.Gorse = "error"
			return nil, stats, 0, ErrGorseUnavailable
		}
		if err == nil && len(recommendIDs) > 0 {
			// 排除全部已标注作品并按推荐顺序去重。
			var filteredIDs []string
			seenIDs := make(map[string]bool, len(recommendIDs))
			for _, id := range recommendIDs {
				if _, rated := ratedMap[id]; !rated && !seenIDs[id] {
					filteredIDs = append(filteredIDs, id)
					seenIDs[id] = true
				}
			}
			if len(filteredIDs) > 0 {
				if err := s.db.WithContext(ctx).Where("normalized_id IN ? AND is_hidden = ?", filteredIDs, false).Find(&gorseMovies).Error; err != nil {
					return nil, stats, 0, err
				}
				if len(gorseMovies) > 0 {
					// 按照推荐顺序重排
					orderMap := make(map[string]int, len(filteredIDs))
					for i, id := range filteredIDs {
						orderMap[id] = i
					}
					sort.SliceStable(gorseMovies, func(i, j int) bool {
						return orderMap[gorseMovies[i].NormalizedID] < orderMap[gorseMovies[j].NormalizedID]
					})
					details.Algorithm, details.Gorse = "gorse", "used"
				}
			}
		}
	}

	details.Algorithm = "gorse"
	details.Candidates = len(gorseMovies)
	totalCandidates := int64(len(gorseMovies))
	if page > len(gorseMovies)/pageSize+1 {
		return []catalog.Movie{}, stats, totalCandidates, nil
	}
	offset := (page - 1) * pageSize
	if offset >= len(gorseMovies) {
		return []catalog.Movie{}, stats, totalCandidates, nil
	}
	movies := gorseMovies[offset:min(offset+pageSize, len(gorseMovies))]
	if err := s.catalogRepo.Populate(ctx, movies, false); err != nil {
		return nil, stats, 0, err
	}
	return movies, stats, totalCandidates, nil
}

// RecordFeedback 记录不含用户或影片标识的反馈汇总。
func (s *Service) RecordFeedback(ctx context.Context, origin, action string, stats Stats) {
	details := logbook.RecommendationDetails{Origin: origin, Action: strings.ToLower(strings.TrimSpace(action)), Liked: stats.Liked, Disliked: stats.Disliked, Uncertain: stats.Uncertain}
	logbook.New(s.db, os.Stdout).Record(ctx, logbook.RecommendationEntry("recommendation_feedback", "info", details))
}
