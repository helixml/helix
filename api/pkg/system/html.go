package system

import (
	"fmt"
	"io"
	"net/http"

	"jaytaylor.com/html2text"
)

// ExtractTextFromURL takes a URL as input, downloads the HTML content, and returns the plain text.
func ExtractHTMLFromURL(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 response status: %d", resp.StatusCode)
	}
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func ExtractTextFromHTML(htmlString string) (string, error) {
	return html2text.FromString(htmlString, html2text.Options{PrettyTables: true})
}

func ExtractTextFromURL(url string) (string, error) {
	html, err := ExtractHTMLFromURL(url)
	if err != nil {
		return "", err
	}
	return ExtractTextFromHTML(html)
}
