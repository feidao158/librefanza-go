package recommendation

import (
	"time"
)

// UserFeedback 用户对特定番号影片的人工标注/反馈
type UserFeedback struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       string    `json:"user_id" gorm:"size:128;not null;index:user_feedbacks_user_idx"`
	NormalizedID string    `json:"normalized_id" gorm:"size:128;not null;index:user_feedbacks_movie_idx"`
	FeedbackType string    `json:"feedback_type" gorm:"size:32;not null"` // like, dislike, uncertain
	Score        float64   `json:"score" gorm:"default:0.0"`
	Category     string    `json:"category" gorm:"size:64;default:''"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (UserFeedback) TableName() string {
	return "user_feedbacks"
}

// UserRecommendationProfile 用户的推荐画像与特征参数
type UserRecommendationProfile struct {
	ID             uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID         string    `json:"user_id" gorm:"size:128;not null;uniqueIndex"`
	FeedbackCount  int       `json:"feedback_count" gorm:"default:0"`
	LikeCount      int       `json:"like_count" gorm:"default:0"`
	DislikeCount   int       `json:"dislike_count" gorm:"default:0"`
	UncertainCount int       `json:"uncertain_count" gorm:"default:0"`
	ParametersJSON string    `json:"parameters_json" gorm:"type:text;not null"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (UserRecommendationProfile) TableName() string {
	return "user_recommendation_profiles"
}

// PreferenceParameters 用户的细粒度偏好参数（演员、类别、厂牌、导演权重）
type PreferenceParameters struct {
	Actresses map[string]float64 `json:"actresses"`
	Genres    map[string]float64 `json:"genres"`
	Makers    map[string]float64 `json:"makers"`
	Labels    map[string]float64 `json:"labels"`
	Directors map[string]float64 `json:"directors"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// Stats 用户标注统计与猜你喜欢就绪状态
type Stats struct {
	Total       int  `json:"total"`
	Liked       int  `json:"liked"`
	Disliked    int  `json:"disliked"`
	Uncertain   int  `json:"uncertain"`
	MinRequired int  `json:"min_required"`
	Ready       bool `json:"ready"`
}

// FeedbackInput 提交反馈入参
type FeedbackInput struct {
	NormalizedID string `json:"normalized_id"`
	Action       string `json:"action"` // like, dislike, uncertain
}
