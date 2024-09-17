package readability

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	readability "github.com/go-shiori/go-readability"
)

type Parser interface {
	Parse(ctx context.Context, content, url string) (*Article, error)
}

type Article struct {
	Title   string
	Byline  string
	Excerpt string
	Content string
}

func NewParser() Parser {
	parser := readability.NewParser()

	return &DefaultParser{parser: &parser}
}

type DefaultParser struct {
	parser *readability.Parser
}

func (p *DefaultParser) Parse(ctx context.Context, content, u string) (*Article, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	article, err := p.parser.Parse(strings.NewReader(content), parsedURL)
	if err != nil {
		return nil, err
	}

	fmt.Println("title ", article.Title)
	fmt.Println("byline ", article.Byline)
	fmt.Println("excerpt ", article.Excerpt)

	return &Article{
		Title:   article.Title,
		Byline:  article.Byline,
		Excerpt: article.Excerpt,
		Content: article.Content,
	}, nil
}
