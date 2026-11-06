package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

type ActressRecord struct {
	ID       int64
	Name     string
	ImageURL string
	RawJSON  string
}

func CrawlActresses(ctx context.Context, settings ClientSettings, offset int, consume func(ActressRecord) error) error {
	if settings.FanzaAPIID == "" || settings.FanzaAffiliateID == "" {
		return errors.New("Fanza credentials are not configured")
	}
	providers, err := NewProviders(settings)
	if err != nil {
		return err
	}
	provider := providers[0].(*httpProvider)
	if offset < 1 {
		offset = 1
	}
	for offset <= 1000000 {
		endpoint := provider.base.ResolveReference(&url.URL{Path: provider.base.Path + "ActressSearch"})
		values := endpoint.Query()
		for key, value := range map[string]string{"api_id": settings.FanzaAPIID, "affiliate_id": settings.FanzaAffiliateID, "hits": "100", "offset": strconv.Itoa(offset), "sort": "id", "output": "json"} {
			values.Set(key, value)
		}
		endpoint.RawQuery = values.Encode()
		body, _, err := provider.get(ctx, endpoint.String(), nil)
		if err != nil {
			return err
		}
		var response struct {
			Result struct {
				Actresses     []json.RawMessage `json:"actress"`
				ResultCount   json.RawMessage   `json:"result_count"`
				TotalCount    json.RawMessage   `json:"total_count"`
				FirstPosition json.RawMessage   `json:"first_position"`
			} `json:"result"`
		}
		if json.Unmarshal(body, &response) != nil {
			return errors.New("invalid Fanza actress response")
		}
		for _, raw := range response.Result.Actresses {
			var entry struct {
				ID       json.RawMessage `json:"id"`
				Name     string          `json:"name"`
				ImageURL struct {
					Large string `json:"large"`
					Small string `json:"small"`
				} `json:"imageURL"`
			}
			if json.Unmarshal(raw, &entry) != nil {
				continue
			}
			record := ActressRecord{ID: int64(jsonNumber(entry.ID)), Name: strings.TrimSpace(entry.Name), ImageURL: entry.ImageURL.Large, RawJSON: string(raw)}
			if record.ImageURL == "" {
				record.ImageURL = entry.ImageURL.Small
			}
			if record.ID > 0 && record.Name != "" {
				if err := consume(record); err != nil {
					return err
				}
			}
		}
		count, total := int(jsonNumber(response.Result.ResultCount)), int(jsonNumber(response.Result.TotalCount))
		if count == 0 || offset+count > total {
			return nil
		}
		next := int(jsonNumber(response.Result.FirstPosition)) + count
		if next <= offset {
			return errors.New("invalid actress pagination")
		}
		offset = next
	}
	return errors.New("actress pagination limit reached")
}

func CrawlMGStage(ctx context.Context, settings ClientSettings, sink Sink) error {
	if settings.MGStageSeriesURL == "" {
		return errors.New("MGStage series URL is not configured")
	}
	seriesURL, err := url.Parse(settings.MGStageSeriesURL)
	if err != nil || (seriesURL.Scheme != "http" && seriesURL.Scheme != "https") || seriesURL.Host == "" {
		return errors.New("invalid MGStage series URL")
	}
	providers, err := NewProviders(settings)
	if err != nil {
		return err
	}
	provider := providers[2].(*httpProvider)
	body, _, err := provider.get(ctx, seriesURL.String(), nil)
	if err != nil {
		return err
	}
	for _, series := range strings.Fields(string(body)) {
		items, err := provider.searchMGStage(ctx, strings.ToUpper(series))
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := sink.Save(ctx, item); err != nil {
				return err
			}
		}
	}
	return nil
}
