package types

import "time"

// WebPage is a fetched public web page cached for the web search API. Rows
// are shared across organizations: they hold public web content keyed only by
// URL, never anything a caller supplied beyond the URL itself.
type WebPage struct {
	// ID is websearch.DocumentID(URL), the id handed out in search results.
	ID string `json:"id" gorm:"primaryKey"`
	// URL is the requested URL, normalised by websearch.NormalizeURL.
	URL         string    `json:"url" gorm:"not null;uniqueIndex"`
	FinalURL    string    `json:"final_url"`
	StatusCode  int       `json:"status_code"`
	ContentType string    `json:"content_type"`
	Body        []byte    `json:"-" gorm:"type:bytea"`
	FetchedAt   time.Time `json:"fetched_at"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}
