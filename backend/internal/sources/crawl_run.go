package sources

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
)

type CrawlRequest struct {
	Source      string `json:"source"`
	Mode        string `json:"mode"`
	StartPage   int    `json:"start_page,omitempty"`
	EndPage     int    `json:"end_page,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	EndDate     string `json:"end_date,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
	StartNumber int    `json:"start_number,omitempty"`
	EndNumber   int    `json:"end_number,omitempty"`
	NumberWidth int    `json:"number_width,omitempty"`
}

type CrawlProgress struct {
	NextPage   int `json:"next_page"`
	NextNumber int `json:"next_number"`
	Processed  int `json:"processed"`
	Undated    int `json:"undated"`
	Skipped    int `json:"skipped"`
}

type CrawlSource interface {
	Page(context.Context, int) ([]Parsed, error)
	Search(context.Context, string) ([]Parsed, error)
}

var ErrSourceRetired = errors.New("source retired")

func (r CrawlRequest) Validate() error {
	switch r.Source {
	case "avmoo", "javlibrary", "javbus":
		return ErrSourceRetired
	case "jable", "libredmm", "javdb", "javbooks", "javquery":
	default:
		return errors.New("invalid crawl source")
	}
	switch r.Mode {
	case "incremental", "full":
	case "pages":
		if r.StartPage < 1 || r.EndPage < r.StartPage || r.EndPage > 1000000 {
			return errors.New("invalid page range")
		}
	case "dates":
		start, err := time.Parse("2006-01-02", r.StartDate)
		end, endErr := time.Parse("2006-01-02", r.EndDate)
		if err != nil || endErr != nil || end.Before(start) || r.Source == "jable" {
			return errors.New("invalid date range")
		}
	case "ids":
		if !regexp.MustCompile(`^[A-Za-z0-9]{1,24}$`).MatchString(r.Prefix) || !catalog.IsProductID(r.Prefix+"-001") || r.StartNumber < 0 || r.EndNumber < r.StartNumber || r.EndNumber > 9999999999 || r.EndNumber-r.StartNumber > 100000 || r.NumberWidth < 0 || r.NumberWidth > 10 {
			return errors.New("invalid product ID range")
		}
	default:
		return errors.New("invalid crawl mode")
	}
	return nil
}

func RunCrawl(ctx context.Context, request CrawlRequest, source CrawlSource, sink Sink, progress CrawlProgress, save func(CrawlProgress) error) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Mode == "ids" {
		next := progress.NextNumber
		if next < request.StartNumber {
			next = request.StartNumber
		}
		for number := next; number <= request.EndNumber; number++ {
			id := fmt.Sprintf("%s-%0*d", strings.ToUpper(request.Prefix), request.NumberWidth, number)
			items, err := source.Search(ctx, id)
			if err != nil {
				return &CrawlError{PositionType: "number", Position: number, Stage: "fetch", Err: err}
			}
			for _, item := range items {
				if err := sink.Save(ctx, item); err != nil {
					return &CrawlError{PositionType: "number", Position: number, Stage: "save", Err: err}
				}
				progress.Processed++
			}
			progress.NextNumber = number + 1
			if err := save(progress); err != nil {
				return &CrawlError{PositionType: "number", Position: number, Stage: "progress", Err: err}
			}
		}
		return nil
	}
	startPage, endPage := 1, 1000000
	if request.Mode == "incremental" {
		endPage = 3
	} else if request.Mode == "pages" {
		startPage, endPage = request.StartPage, request.EndPage
	}
	page := startPage
	if progress.NextPage > startPage {
		page = progress.NextPage - 1
	}
	for ; page <= endPage; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var items []Parsed
		var skipped int
		var err error
		if detailed, ok := source.(interface {
			PageWithStats(context.Context, int) ([]Parsed, int, error)
		}); ok {
			items, skipped, err = detailed.PageWithStats(ctx, page)
		} else {
			items, err = source.Page(ctx, page)
		}
		if err != nil {
			return &CrawlError{PositionType: "page", Position: page, Stage: "fetch", Err: err}
		}
		if len(items) == 0 && skipped == 0 {
			return nil
		}
		if page >= progress.NextPage {
			progress.Skipped += skipped
		}
		for _, item := range items {
			if request.Mode == "dates" {
				if item.Item.ReleaseDate == nil {
					if page >= progress.NextPage {
						progress.Undated++
					}
					continue
				}
				date := item.Item.ReleaseDate.Format("2006-01-02")
				if date < request.StartDate || date > request.EndDate {
					continue
				}
			}
			if err := sink.Save(ctx, item); err != nil {
				return &CrawlError{PositionType: "page", Position: page, Stage: "save", Err: err}
			}
			if page >= progress.NextPage {
				progress.Processed++
			}
		}
		progress.NextPage = page + 1
		if err := save(progress); err != nil {
			return &CrawlError{PositionType: "page", Position: page, Stage: "progress", Err: err}
		}
	}
	return nil
}
