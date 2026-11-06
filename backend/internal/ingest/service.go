package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/libredmm/librefanza/backend/internal/cache"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

type page struct {
	ID        uint64
	Source    string
	URL       string
	URLHash   string
	RawHTML   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (page) TableName() string { return "source_pages" }

type castRecord struct {
	ID      uint64
	MovieID uint64
	FanzaID *int64
	Name    string
}

func (castRecord) TableName() string { return "movie_cast" }

func (s *Service) SaveActress(ctx context.Context, record sources.ActressRecord) error {
	if record.ID <= 0 || strings.TrimSpace(record.Name) == "" {
		return errors.New("invalid actress record")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actress := catalog.Actress{FanzaID: &record.ID, Name: strings.TrimSpace(record.Name), SearchName: strings.ToLower(strings.TrimSpace(record.Name)), ImageURL: record.ImageURL, RawJSON: record.RawJSON}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "fanza_id"}}, DoUpdates: clause.AssignmentColumns([]string{"name", "search_name", "image_url", "raw_json", "updated_at"})}).Create(&actress).Error; err != nil {
			return err
		}
		return cache.BumpVersion(tx)
	})
}

func (s *Service) Save(ctx context.Context, parsed sources.Parsed) error {
	item := parsed.Item
	if item.Source == "" || item.ExternalID == "" || len(item.ExternalID) > 191 || !catalog.IsProductID(item.NormalizedID) || item.Title == "" {
		return errors.New("invalid source item")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous catalog.SourceItem
		err := tx.Where("source = ? AND external_id = ?", item.Source, item.ExternalID).First(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if item.Source == "fanza" && item.Description == "" {
			item.Description = previous.Description
		}
		movie := catalog.Movie{NormalizedID: item.NormalizedID, CompressedID: catalog.CompressProductID(item.NormalizedID)}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&movie).Error; err != nil {
			return err
		}
		ids := []string{item.NormalizedID}
		if previous.NormalizedID != "" && previous.NormalizedID != item.NormalizedID {
			ids = append(ids, previous.NormalizedID)
		}
		sort.Strings(ids)
		var locked []catalog.Movie
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("normalized_id IN ?", ids).Order("normalized_id").Find(&locked).Error; err != nil {
			return err
		}
		if parsed.RawHTML != "" {
			hash := sha256.Sum256([]byte(item.SourceURL))
			entry := page{Source: item.Source, URL: item.SourceURL, URLHash: hex.EncodeToString(hash[:]), RawHTML: parsed.RawHTML}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "url_hash"}}, DoUpdates: clause.AssignmentColumns([]string{"raw_html", "updated_at"})}).Create(&entry).Error; err != nil {
				return err
			}
			if err := tx.Where("url_hash = ?", entry.URLHash).First(&entry).Error; err != nil {
				return err
			}
			item.SourcePageID = &entry.ID
		}
		columns := []string{"normalized_id", "source_page_id", "title", "release_date", "description", "source_url", "cover_image_url", "thumbnail_image_url", "metadata_json", "raw_json", "updated_at"}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source"}, {Name: "external_id"}}, DoUpdates: clause.AssignmentColumns(columns)}).Create(&item).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := deriveMovie(tx, id); err != nil {
				return err
			}
		}
		return cache.BumpVersion(tx)
	})
}

func (s *Service) SetPriority(ctx context.Context, itemID uint64, priority int) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item catalog.SourceItem
		if err := tx.First(&item, itemID).Error; err != nil {
			return err
		}
		if item.Source != "fanza" {
			return errors.New("priority is only supported for Fanza")
		}
		var movie catalog.Movie
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("normalized_id = ?", item.NormalizedID).First(&movie).Error; err != nil {
			return err
		}
		if err := tx.Model(&item).Update("priority", priority).Error; err != nil {
			return err
		}
		if err := deriveMovie(tx, item.NormalizedID); err != nil {
			return err
		}
		return cache.BumpVersion(tx)
	})
}

func (s *Service) RebuildMovies(ctx context.Context) error {
	var after uint64
	for {
		var movies []catalog.Movie
		if err := s.db.WithContext(ctx).Select("id", "normalized_id").Where("id > ?", after).Order("id").Limit(100).Find(&movies).Error; err != nil {
			return err
		}
		if len(movies) == 0 {
			return nil
		}
		for _, movie := range movies {
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var locked catalog.Movie
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, movie.ID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil
					}
					return err
				}
				if err := deriveMovie(tx, locked.NormalizedID); err != nil {
					return err
				}
				return cache.BumpVersion(tx)
			}); err != nil {
				return err
			}
			after = movie.ID
		}
	}
}

func deriveMovie(tx *gorm.DB, id string) error {
	var movie catalog.Movie
	if err := tx.Where("normalized_id = ?", id).First(&movie).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var items []catalog.SourceItem
	if err := tx.Where("normalized_id = ?", id).Find(&items).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return tx.Delete(&movie).Error
	}
	catalog.SortSourceItems(items)
	preferred := items[0]
	if err := tx.Model(&movie).Updates(map[string]any{"compressed_id": catalog.CompressProductID(id), "release_date": preferred.ReleaseDate, "updated_at": time.Now().UTC()}).Error; err != nil {
		return err
	}
	if err := tx.Where("movie_id = ?", movie.ID).Delete(&castRecord{}).Error; err != nil {
		return err
	}
	var metadata sources.Metadata
	if preferred.MetadataJSON != "" {
		if err := json.Unmarshal([]byte(preferred.MetadataJSON), &metadata); err != nil {
			return err
		}
	}
	for _, member := range metadata.Cast {
		if member.Name != "" {
			if err := tx.Create(&castRecord{MovieID: movie.ID, FanzaID: member.FanzaID, Name: member.Name}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
