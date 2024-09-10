package extract

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-tika/tika"
	"golang.org/x/net/html"
)

// TikaExtractor is the default, llamaindex based text extractor
// that can download URLs and uses unstructured.io under the hood
type TikaExtractor struct {
	extractorURL string
	httpClient   *http.Client
}

func NewTikaExtractor(extractorURL string) *TikaExtractor {
	if extractorURL == "" {
		extractorURL = "http://localhost:9998"
	}

	return &TikaExtractor{
		extractorURL: extractorURL,
		httpClient:   http.DefaultClient,
	}
}

func (e *TikaExtractor) Extract(ctx context.Context, extractReq *ExtractRequest) (string, error) {
	if extractReq.URL == "" && len(extractReq.Content) == 0 {
		return "", fmt.Errorf("no URL or content provided")
	}

	client := tika.NewClient(e.httpClient, e.extractorURL)

	parsed, err := client.Parse(ctx, bytes.NewReader(extractReq.Content))
	if err != nil {
		return "", err
	}

	// os.WriteFile("out_raw.html", []byte(parsed), 0o644)

	// Parsed content is returned as a sequence of XHTML SAX events.
	// XHTML is used to express structured content of the document.
	// The overall structure of the generated event stream is (with indenting added for clarity):
	// <html xmlns="http://www.w3.org/1999/xhtml">
	//   <head>
	// 	<title>...</title>
	// </head>
	// <body>
	// 	...
	// </body>
	// </html>
	// Ref: https://tika.apache.org/3.0.0-BETA2/parser.html
	return parsed, nil
	// return e.convertXHTMLToMarkdown(ctx, parsed)
}

// func (e *TikaExtractor) convertXHTMLToMarkdown(ctx context.Context, parsed string) (string, error) {
// 	converter := md.NewConverter("", true, nil)

// 	return converter.ConvertString(parsed)
// }

func (e *TikaExtractor) convertXHTMLToMarkdown(ctx context.Context, parsed string) (string, error) {
	// Create an HTML tokenizer
	tokenizer := html.NewTokenizer(strings.NewReader(parsed))

	var markdown strings.Builder
	var inBody bool
	var inParagraph bool
	var inList bool
	var listType string

	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return markdown.String(), nil
			}
			return "", tokenizer.Err()

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			switch token.Data {
			case "body":
				inBody = true
			case "p":
				if inBody {
					markdown.WriteString("\n\n")
					inParagraph = true
				}
			case "br":
				markdown.WriteString("\n")
			case "h1", "h2", "h3", "h4", "h5", "h6":
				markdown.WriteString("\n\n" + strings.Repeat("#", int(token.Data[1]-'0')) + " ")
			case "ul", "ol":
				inList = true
				listType = token.Data
			case "li":
				if inList {
					markdown.WriteString("\n")
					if listType == "ul" {
						markdown.WriteString("- ")
					} else {
						markdown.WriteString("1. ") // For simplicity, always use '1.' for ordered lists
					}
				}
			}

		case html.EndTagToken:
			token := tokenizer.Token()
			switch token.Data {
			case "body":
				inBody = false
			case "p":
				inParagraph = false
			case "ul", "ol":
				inList = false
				listType = ""
				markdown.WriteString("\n")
			}

		case html.TextToken:
			if inBody {
				text := strings.TrimSpace(string(tokenizer.Text()))
				if text != "" {
					markdown.WriteString(text)
					if inParagraph {
						markdown.WriteString(" ")
					}
				}
			}
		}
	}
}