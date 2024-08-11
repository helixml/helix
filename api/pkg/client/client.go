package client

import "net/http"

// Client is the client for the helix api
type Client struct {
	httpClient *http.Client
	apiKey     string
	url        string
}

func NewClient(url, apiKey string) *Client {
	return &Client{
		httpClient: http.DefaultClient,
		apiKey:     apiKey,
		url:        url,
	}
}
