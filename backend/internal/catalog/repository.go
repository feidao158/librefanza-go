package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/libredmm/librefanza/backend/internal/images"
	"gorm.io/gorm"
)

type Movie struct {
	ID            uint64       `json:"id"`
	NormalizedID  string       `json:"normalized_id"`
	CompressedID  string       `json:"compressed_id"`
	ReleaseDate   *time.Time   `json:"release_date"`
	CoverImageURL string       `json:"cover_image_url"`
	IsHidden      bool         `json:"is_hidden"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	PreferredItem *SourceItem  `json:"preferred_item,omitempty" gorm:"-"`
	Versions      []SourceItem `json:"versions,omitempty" gorm:"-"`
}

type SourceItem struct {
	ID                 uint64     `json:"id"`
	Source             string     `json:"source"`
	ExternalID         string     `json:"external_id"`
	NormalizedID       string     `json:"normalized_id"`
	SourcePageID       *uint64    `json:"source_page_id,omitempty"`
	Priority           int        `json:"priority"`
	Title              string     `json:"title"`
	ReleaseDate        *time.Time `json:"release_date"`
	Description        string     `json:"description"`
	SourceURL          string     `json:"source_url"`
	CoverImageURL      string     `json:"cover_image_url"`
	ThumbnailImageURL  string     `json:"thumbnail_image_url"`
	CoverIsImproved    bool       `json:"-"`
	MetadataJSON       string     `json:"-"`
	RawJSON            string     `json:"-"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CoverImageOverride bool       `json:"-" gorm:"-"`
}

func (item SourceItem) MarshalJSON() ([]byte, error) {
	type sourceItemAlias SourceItem
	originalCover := ""
	if item.Source == "sod" {
		if !item.CoverImageOverride && images.AllowedSODURL(item.CoverImageURL) {
			originalCover = item.CoverImageURL
			item.CoverImageURL = fmt.Sprintf("/api/v1/source-items/%d/images/cover", item.ID)
		}
		if !item.CoverImageOverride && images.AllowedSODURL(item.ThumbnailImageURL) {
			item.ThumbnailImageURL = fmt.Sprintf("/api/v1/source-items/%d/images/thumbnail", item.ID)
		}
	}
	metadata := json.RawMessage(item.MetadataJSON)
	if !json.Valid(metadata) || len(metadata) == 0 || string(metadata) == "null" {
		metadata = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(metadata, &fields) == nil && fields != nil {
		var samples []string
		if json.Unmarshal(fields["sample_image_urls"], &samples) == nil {
			originals := make([]string, len(samples))
			proxied := false
			for i, address := range samples {
				parsed, err := url.Parse(address)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
					samples[i] = ""
					continue
				}
				originals[i] = address
				if images.AllowedSourceURL(item.Source, address) {
					samples[i] = fmt.Sprintf("/api/v1/source-items/%d/images/samples/%d", item.ID, i)
					proxied = true
				}
			}
			fields["sample_image_urls"], _ = json.Marshal(samples)
			if proxied {
				fields["sample_image_original_urls"], _ = json.Marshal(originals)
			}
			metadata, _ = json.Marshal(fields)
		}
	}
	return json.Marshal(struct {
		sourceItemAlias
		Metadata      json.RawMessage `json:"metadata"`
		OriginalCover string          `json:"cover_image_original_url,omitempty"`
	}{sourceItemAlias: sourceItemAlias(item), Metadata: metadata, OriginalCover: originalCover})
}

type Actress struct {
	ID         uint64    `json:"id"`
	FanzaID    *int64    `json:"fanza_id"`
	Name       string    `json:"name"`
	SearchName string    `json:"-"`
	ImageURL   string    `json:"image_url"`
	RawJSON    string    `json:"-"`
	IsHidden   bool      `json:"is_hidden"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type MovieQuery struct {
	Query    string
	Match    string
	Order    string
	Page     int
	PageSize int
}

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Movies(ctx context.Context, query MovieQuery) ([]Movie, int64, error) {
	page, size := pagination(query.Page, query.PageSize)
	db := r.db.WithContext(ctx).Model(&Movie{}).Where("is_hidden = ?", false)
	if query.Query != "" {
		value := escapeLike(strings.ToUpper(strings.TrimSpace(query.Query)))
		if query.Match == "prefix" {
			value += "%"
		} else {
			value = "%" + value + "%"
		}
		db = db.Where("normalized_id LIKE ? ESCAPE '!'", value)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	switch query.Order {
	case "release_date":
		db = db.Order("CASE WHEN release_date IS NULL THEN 1 ELSE 0 END").Order("release_date DESC").Order("normalized_id DESC")
	case "date_added":
		db = db.Order("created_at DESC").Order("normalized_id DESC")
	default:
		db = db.Order("normalized_id ASC")
	}
	var movies []Movie
	if err := db.Offset((page - 1) * size).Limit(size).Find(&movies).Error; err != nil {
		return nil, 0, err
	}
	if err := r.Populate(ctx, movies, false); err != nil {
		return nil, 0, err
	}
	return movies, total, nil
}

func (r *Repository) Movie(ctx context.Context, id string) (Movie, error) {
	var movie Movie
	id = strings.ToUpper(id)
	if IsProductID(id) {
		id = NormalizeProductID(id)
	}
	err := r.db.WithContext(ctx).Where("normalized_id = ? AND is_hidden = ?", id, false).First(&movie).Error
	if err != nil {
		return Movie{}, err
	}
	items := []Movie{movie}
	if err := r.Populate(ctx, items, true); err != nil {
		return Movie{}, err
	}
	if items[0].PreferredItem == nil {
		return Movie{}, gorm.ErrRecordNotFound
	}
	return items[0], nil
}

func (r *Repository) SourceItem(ctx context.Context, id uint64) (SourceItem, error) {
	var item SourceItem
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return SourceItem{}, err
	}
	var movie Movie
	if err := r.db.WithContext(ctx).Where("normalized_id = ? AND is_hidden = ?", item.NormalizedID, false).First(&movie).Error; err != nil {
		return SourceItem{}, err
	}
	items := []SourceItem{item}
	if err := r.redactHiddenActresses(ctx, items); err != nil {
		return SourceItem{}, err
	}
	item = items[0]
	return item, nil
}

// Populate 批量组装影片来源与首选展示数据
func (r *Repository) Populate(ctx context.Context, movies []Movie, includeVersions bool) error {
	ids := make([]string, 0, len(movies))
	for _, movie := range movies {
		ids = append(ids, movie.NormalizedID)
	}
	groups := make(map[string][]SourceItem, len(ids))
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		var items []SourceItem
		if err := r.db.WithContext(ctx).Where("normalized_id IN ?", ids[start:end]).Find(&items).Error; err != nil {
			return err
		}
		if err := r.redactHiddenActresses(ctx, items); err != nil {
			return err
		}
		for _, item := range items {
			groups[item.NormalizedID] = append(groups[item.NormalizedID], item)
		}
	}
	for i := range movies {
		items := groups[movies[i].NormalizedID]
		sort.SliceStable(items, func(a, b int) bool { return preferredBefore(items[a], items[b]) })
		if len(items) != 0 {
			preferred := items[0]
			if movies[i].CoverImageURL != "" {
				preferred.CoverImageURL = movies[i].CoverImageURL
				preferred.ThumbnailImageURL = movies[i].CoverImageURL
				preferred.CoverImageOverride = true
			}
			movies[i].PreferredItem = &preferred
			if includeVersions {
				movies[i].Versions = items
			}
		}
	}
	return nil
}

func (r *Repository) redactHiddenActresses(ctx context.Context, items []SourceItem) error {
	type castMember struct {
		Name    string `json:"name"`
		FanzaID *int64 `json:"fanza_id"`
	}
	var names []string
	var ids []int64
	for _, item := range items {
		var metadata struct {
			Cast []castMember `json:"cast"`
		}
		if json.Unmarshal([]byte(item.MetadataJSON), &metadata) != nil {
			continue
		}
		for _, member := range metadata.Cast {
			if member.Name != "" {
				names = append(names, member.Name)
			}
			if member.FanzaID != nil {
				ids = append(ids, *member.FanzaID)
			}
		}
	}
	hiddenNames := map[string]bool{}
	hiddenIDs := map[int64]bool{}
	for start := 0; start < len(names); start += 500 {
		var actresses []Actress
		if err := r.db.WithContext(ctx).Select("name", "fanza_id").Where("is_hidden = ? AND name IN ?", true, names[start:min(start+500, len(names))]).Find(&actresses).Error; err != nil {
			return err
		}
		for _, actress := range actresses {
			hiddenNames[actress.Name] = true
			if actress.FanzaID != nil {
				hiddenIDs[*actress.FanzaID] = true
			}
		}
	}
	for start := 0; start < len(ids); start += 500 {
		var actresses []Actress
		if err := r.db.WithContext(ctx).Select("name", "fanza_id").Where("is_hidden = ? AND fanza_id IN ?", true, ids[start:min(start+500, len(ids))]).Find(&actresses).Error; err != nil {
			return err
		}
		for _, actress := range actresses {
			hiddenNames[actress.Name] = true
			if actress.FanzaID != nil {
				hiddenIDs[*actress.FanzaID] = true
			}
		}
	}
	if len(hiddenNames) == 0 && len(hiddenIDs) == 0 {
		return nil
	}
	for i := range items {
		var metadata map[string]json.RawMessage
		if json.Unmarshal([]byte(items[i].MetadataJSON), &metadata) != nil {
			continue
		}
		var cast []json.RawMessage
		if json.Unmarshal(metadata["cast"], &cast) != nil {
			continue
		}
		filtered := make([]json.RawMessage, 0, len(cast))
		for _, raw := range cast {
			var member castMember
			if json.Unmarshal(raw, &member) != nil {
				continue
			}
			if hiddenNames[member.Name] || member.FanzaID != nil && hiddenIDs[*member.FanzaID] {
				continue
			}
			filtered = append(filtered, raw)
		}
		metadata["cast"], _ = json.Marshal(filtered)
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		items[i].MetadataJSON = string(encoded)
	}
	return nil
}

func preferredBefore(a, b SourceItem) bool {
	rank := map[string]int{"fanza": 0, "sod": 1, "mgstage": 2, "javlibrary": 3, "fc2": 4}
	ra, oka := rank[a.Source]
	rb, okb := rank[b.Source]
	if !oka {
		ra = 5
	}
	if !okb {
		rb = 5
	}
	if ra != rb {
		return ra < rb
	}
	if a.Source == "fanza" && a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.Source == "fanza" {
		if a.ReleaseDate == nil && b.ReleaseDate != nil {
			return false
		}
		if a.ReleaseDate != nil && b.ReleaseDate == nil {
			return true
		}
		if a.ReleaseDate != nil && b.ReleaseDate != nil && !a.ReleaseDate.Equal(*b.ReleaseDate) {
			return a.ReleaseDate.After(*b.ReleaseDate)
		}
	}
	return a.ID < b.ID
}

func SortSourceItems(items []SourceItem) {
	sort.SliceStable(items, func(a, b int) bool { return preferredBefore(items[a], items[b]) })
}

func (r *Repository) Actresses(ctx context.Context, query string, order string, page, size int) ([]Actress, int64, error) {
	page, size = pagination(page, size)
	db := r.db.WithContext(ctx).Model(&Actress{}).Where("is_hidden = ?", false)
	if query != "" {
		db = db.Where("search_name LIKE ? ESCAPE '!'", "%"+escapeLike(strings.ToLower(strings.TrimSpace(query)))+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if order == "name" {
		db = db.Order("search_name ASC").Order("id ASC")
	} else {
		db = db.Order("fanza_id DESC").Order("id DESC")
	}
	var actresses []Actress
	err := db.Offset((page - 1) * size).Limit(size).Find(&actresses).Error
	return actresses, total, err
}

func (r *Repository) Actress(ctx context.Context, id uint64) (Actress, error) {
	var actress Actress
	err := r.db.WithContext(ctx).Where("id = ? AND is_hidden = ?", id, false).First(&actress).Error
	return actress, err
}

func (r *Repository) Filmography(ctx context.Context, actressID uint64, solo bool, page, size int) ([]Movie, int64, error) {
	actress, err := r.Actress(ctx, actressID)
	if err != nil {
		return nil, 0, err
	}
	return r.filmography(ctx, actress, solo, page, size)
}

func (r *Repository) FilmographyForName(ctx context.Context, name string, solo bool, page, size int) (Actress, []Movie, int64, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 512 {
		return Actress{}, nil, 0, gorm.ErrRecordNotFound
	}
	var actress Actress
	err := r.db.WithContext(ctx).Where("name = ?", name).Order("fanza_id ASC").First(&actress).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		actress = Actress{Name: name}
	} else if err != nil {
		return Actress{}, nil, 0, err
	}
	if actress.IsHidden {
		return Actress{}, nil, 0, gorm.ErrRecordNotFound
	}
	movies, total, err := r.filmography(ctx, actress, solo, page, size)
	return actress, movies, total, err
}

func (r *Repository) filmography(ctx context.Context, actress Actress, solo bool, page, size int) ([]Movie, int64, error) {
	var hiddenCount int64
	if err := r.db.WithContext(ctx).Model(&Actress{}).Where("name = ? AND is_hidden = ?", actress.Name, true).Count(&hiddenCount).Error; err != nil {
		return nil, 0, err
	}
	if hiddenCount > 0 {
		return nil, 0, gorm.ErrRecordNotFound
	}
	db := r.db.WithContext(ctx).Table("movie_cast").Distinct("movie_id").Where("name = ?", actress.Name)
	if actress.FanzaID != nil {
		db = r.db.WithContext(ctx).Table("movie_cast").Distinct("movie_id").Where("name = ? OR fanza_id = ?", actress.Name, *actress.FanzaID)
	}
	var ids []uint64
	if err := db.Pluck("movie_id", &ids).Error; err != nil {
		return nil, 0, err
	}
	var movies []Movie
	for start := 0; start < len(ids); start += 500 {
		batch := ids[start:min(start+500, len(ids))]
		if solo {
			var soloIDs []uint64
			if err := r.db.WithContext(ctx).Table("movie_cast").Where("movie_id IN ?", batch).Group("movie_id").Having("COUNT(*) = 1").Pluck("movie_id", &soloIDs).Error; err != nil {
				return nil, 0, err
			}
			batch = soloIDs
		}
		if len(batch) == 0 {
			continue
		}
		var items []Movie
		if err := r.db.WithContext(ctx).Where("id IN ? AND is_hidden = ?", batch, false).Find(&items).Error; err != nil {
			return nil, 0, err
		}
		movies = append(movies, items...)
	}
	sort.Slice(movies, func(i, j int) bool { return movies[i].NormalizedID < movies[j].NormalizedID })
	total := int64(len(movies))
	page, size = pagination(page, size)
	start := (page - 1) * size
	if start >= len(movies) {
		return []Movie{}, total, nil
	}
	movies = movies[start:min(start+size, len(movies))]
	if err := r.Populate(ctx, movies, false); err != nil {
		return nil, 0, err
	}
	return movies, total, nil
}

func pagination(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 30
	}
	return page, min(size, 100)
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return replacer.Replace(value)
}

var ErrNotFound = errors.New("not found")
