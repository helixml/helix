package text

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const (
	MAX_HEADING_LENGTH                 = 7
	MAX_HEADING_CONTENT_LENGTH         = 200
	MAX_HEADING_UNDERLINE_LENGTH       = 200
	MAX_HTML_HEADING_ATTRIBUTES_LENGTH = 100
	MAX_LIST_ITEM_LENGTH               = 200
	MAX_NESTED_LIST_ITEMS              = 6
	MAX_LIST_INDENT_SPACES             = 7
	MAX_BLOCKQUOTE_LINE_LENGTH         = 200
	MAX_BLOCKQUOTE_LINES               = 15
	MAX_CODE_BLOCK_LENGTH              = 1500
	MAX_CODE_LANGUAGE_LENGTH           = 20
	MAX_INDENTED_CODE_LINES            = 20
	MAX_TABLE_CELL_LENGTH              = 200
	MAX_TABLE_ROWS                     = 20
	MAX_HTML_TABLE_LENGTH              = 2000
	MIN_HORIZONTAL_RULE_LENGTH         = 3
	MAX_SENTENCE_LENGTH                = 400
	MAX_QUOTED_TEXT_LENGTH             = 300
	MAX_PARENTHETICAL_CONTENT_LENGTH   = 200
	MAX_NESTED_PARENTHESES             = 5
	MAX_MATH_INLINE_LENGTH             = 100
	MAX_MATH_BLOCK_LENGTH              = 500
	MAX_PARAGRAPH_LENGTH               = 1000
	MAX_STANDALONE_LINE_LENGTH         = 800
	MAX_HTML_TAG_ATTRIBUTES_LENGTH     = 100
	MAX_HTML_TAG_CONTENT_LENGTH        = 1000
	LOOKAHEAD_RANGE                    = 100 // Number of characters to look ahead for a sentence boundary
)

var (
	AVOID_AT_START        = `[\\s\\]})>,']`
	PUNCTUATION           = `[.!?…]|\\.{3}|[\\u2026\\u2047-\\u2049]|[\\p{Emoji_Presentation}\\p{Extended_Pictographic}]`
	QUOTE_END             = `(?:'(?=\` + "`" + `)|''(?=\` + `\` + `))`
	SENTENCE_END          = `(?:` + PUNCTUATION + `(?<!` + AVOID_AT_START + `(?=` + PUNCTUATION + `))|` + QUOTE_END + `)(?=\\S|$)`
	SENTENCE_BOUNDARY     = `(?:` + SENTENCE_END + `|(?=[\\r\\n]|$))`
	LOOKAHEAD_PATTERN     = `(?:(?!` + SENTENCE_END + `).){1,` + fmt.Sprint(LOOKAHEAD_RANGE) + `}` + SENTENCE_END
	NOT_PUNCTUATION_SPACE = `(?!` + PUNCTUATION + `\\s)`
	SENTENCE_PATTERN      = NOT_PUNCTUATION_SPACE + `(?:[^\\r\\n]{1,{MAX_LENGTH}}` + SENTENCE_BOUNDARY + `|[^\\r\\n]{1,{MAX_LENGTH}}(?=` + PUNCTUATION + `|` + QUOTE_END + `)(?:` + LOOKAHEAD_PATTERN + `)?)` + AVOID_AT_START + `*`
)

var pattern = `(?m)` + // Enable multiline mode
	`(` +
	// 1. Headings (Setext-style, Markdown, and HTML-style, with length constraints)
	`(?:^(?:[#*=-]{1,` + fmt.Sprint(MAX_HEADING_LENGTH) + `}|\w[^\r\n]{0,` + fmt.Sprint(MAX_HEADING_CONTENT_LENGTH) + `}\r?\n[-=]{2,` + fmt.Sprint(MAX_HEADING_UNDERLINE_LENGTH) + `}|<h[1-6][^>]{0,` + fmt.Sprint(MAX_HTML_HEADING_ATTRIBUTES_LENGTH) + `}>)[^\r\n]{1,` + fmt.Sprint(MAX_HEADING_CONTENT_LENGTH) + `}(?:</h[1-6]>)?(?:\r?\n|$))` +
	`|` +
	// New pattern for citations
	`(?:\[[0-9]+\][^\r\n]{1,` + fmt.Sprint(MAX_STANDALONE_LINE_LENGTH) + `})` +
	`|` +
	// 2. List items (bulleted, numbered, lettered, or task lists, including nested, up to three levels, with length constraints)
	`(?:(?:^|\r?\n)[ \t]{0,3}(?:[-*+•]|\d{1,3}\.\w\.|\[[ xX]\])[ \t]+` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_LIST_ITEM_LENGTH)) +
	`(?:(?:\r?\n[ \t]{2,5}(?:[-*+•]|\d{1,3}\.\w\.|\[[ xX]\])[ \t]+` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_LIST_ITEM_LENGTH)) + `){0,` + fmt.Sprint(MAX_NESTED_LIST_ITEMS) + `}` +
	`(?:\r?\n[ \t]{4,` + fmt.Sprint(MAX_LIST_INDENT_SPACES) + `}(?:[-*+•]|\d{1,3}\.\w\.|\[[ xX]\])[ \t]+` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_LIST_ITEM_LENGTH)) + `){0,` + fmt.Sprint(MAX_NESTED_LIST_ITEMS) + `})?)` +
	`|` +
	// 3. Block quotes (including nested quotes and citations, up to three levels, with length constraints)
	`(?:(?:^>(?:>|\s{2,}){0,2}` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_BLOCKQUOTE_LINE_LENGTH)) + `\r?\n?){1,` + fmt.Sprint(MAX_BLOCKQUOTE_LINES) + `})` +
	`|` +
	// 4. Code blocks (fenced, indented, or HTML pre/code tags, with length constraints)
	`(?:(?:^|\r?\n)(?:` + "`" + "`" + "`" + `|~~~)(?:\w{0,` + fmt.Sprint(MAX_CODE_LANGUAGE_LENGTH) + `})?\r?\n[\s\S]{0,` + fmt.Sprint(MAX_CODE_BLOCK_LENGTH) + `}?(?:` + "`" + "`" + "`" + `|~~~)\r?\n?` +
	`|(?:(?:^|\r?\n)(?: {4}|\t)[^\r\n]{0,` + fmt.Sprint(MAX_LIST_ITEM_LENGTH) + `}(?:\r?\n(?: {4}|\t)[^\r\n]{0,` + fmt.Sprint(MAX_LIST_ITEM_LENGTH) + `}){0,` + fmt.Sprint(MAX_INDENTED_CODE_LINES) + `}\r?\n?)` +
	`|(?:<pre>(?:<code>)?[\s\S]{0,` + fmt.Sprint(MAX_CODE_BLOCK_LENGTH) + `}?(?:</code>)?</pre>))` +
	`|` +
	// 5. Tables (Markdown, grid tables, and HTML tables, with length constraints)
	`(?:(?:^|\r?\n)(?:\|[^\r\n]{0,` + fmt.Sprint(MAX_TABLE_CELL_LENGTH) + `}\|(?:\r?\n\|[-:]{1,` + fmt.Sprint(MAX_TABLE_CELL_LENGTH) + `}\|){0,1}(?:\r?\n\|[^\r\n]{0,` + fmt.Sprint(MAX_TABLE_CELL_LENGTH) + `}\|){0,` + fmt.Sprint(MAX_TABLE_ROWS) + `}` +
	`|<table>[\s\S]{0,` + fmt.Sprint(MAX_HTML_TABLE_LENGTH) + `}?</table>))` +
	`|` +
	// 6. Horizontal rules (Markdown and HTML hr tag)
	`(?:^(?:[-*_]){` + fmt.Sprint(MIN_HORIZONTAL_RULE_LENGTH) + `,}\s*$|<hr\s*/?>)` +
	`|` +
	// 10. Standalone lines or phrases (including single-line blocks and HTML elements, with length constraints)
	`(?!` + AVOID_AT_START + `)(?:^(?:<[a-zA-Z][^>]{0,` + fmt.Sprint(MAX_HTML_TAG_ATTRIBUTES_LENGTH) + `}>)?` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_STANDALONE_LINE_LENGTH)) + `(?:</[a-zA-Z]+>)?(?:\r?\n|$))` +
	`|` +
	// 7. Sentences or phrases ending with punctuation (including ellipsis and Unicode punctuation)
	`(?!` + AVOID_AT_START + `)` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_SENTENCE_LENGTH)) +
	`|` +
	// 8. Quoted text, parenthetical phrases, or bracketed content (with length constraints)
	`(?:` +
	`(?<!\w)"""[^"]{0,` + fmt.Sprint(MAX_QUOTED_TEXT_LENGTH) + `}"""(?!\w)` +
	`|(?<!\w)(?:['"` + "`" + `'""])[^\r\n]{0,` + fmt.Sprint(MAX_QUOTED_TEXT_LENGTH) + `}\1(?!\w)` +
	`|(?<!\w)` + "`" + `[^\r\n]{0,` + fmt.Sprint(MAX_QUOTED_TEXT_LENGTH) + `}'(?!\w)` +
	`|(?<!\w)` + "`" + "`" + `[^\r\n]{0,` + fmt.Sprint(MAX_QUOTED_TEXT_LENGTH) + `}''(?!\w)` +
	`|\([^\r\n()]{0,` + fmt.Sprint(MAX_PARENTHETICAL_CONTENT_LENGTH) + `}(?:\([^\r\n()]{0,` + fmt.Sprint(MAX_PARENTHETICAL_CONTENT_LENGTH) + `}\)[^\r\n()]{0,` + fmt.Sprint(MAX_PARENTHETICAL_CONTENT_LENGTH) + `}){0,` + fmt.Sprint(MAX_NESTED_PARENTHESES) + `}\)` +
	`|\[[^\r\n\[\]]{0,` + fmt.Sprint(MAX_PARENTHETICAL_CONTENT_LENGTH) + `}(?:\[[^\r\n\[\]]{0,` + fmt.Sprint(MAX_PARENTHETICAL_CONTENT_LENGTH) + `}\][^\r\n\[\]]{0,` + fmt.Sprint(MAX_PARENTHETICAL_CONTENT_LENGTH) + `}){0,` + fmt.Sprint(MAX_NESTED_PARENTHESES) + `}\]` +
	`|\$[^\r\n$]{0,` + fmt.Sprint(MAX_MATH_INLINE_LENGTH) + `}\$` +
	`|` + "`" + `[^` + "`" + `\r\n]{0,` + fmt.Sprint(MAX_MATH_INLINE_LENGTH) + `}` + "`" + `` +
	`)` +
	`|` +
	// 9. Paragraphs (with length constraints)
	`(?!` + AVOID_AT_START + `)(?:(?:^|\r?\n\r?\n)(?:<p>)?` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_PARAGRAPH_LENGTH)) + `(?:</p>)?(?=\r?\n\r?\n|$))` +
	`|` +
	// 11. HTML-like tags and their content (including self-closing tags and attributes, with length constraints)
	`(?:<[a-zA-Z][^>]{0,` + fmt.Sprint(MAX_HTML_TAG_ATTRIBUTES_LENGTH) + `}(?:>[\s\S]{0,` + fmt.Sprint(MAX_HTML_TAG_CONTENT_LENGTH) + `}?</[a-zA-Z]+>|\s*/>))` +
	`|` +
	// 12. LaTeX-style math expressions (inline and block, with length constraints)
	`(?:(?:\$\$[\s\S]{0,` + fmt.Sprint(MAX_MATH_BLOCK_LENGTH) + `}?\$\$)|(?:\$[^\$\r\n]{0,` + fmt.Sprint(MAX_MATH_INLINE_LENGTH) + `}\$))` +
	`|` +
	// 14. Fallback for any remaining content (with length constraints)
	`(?!` + AVOID_AT_START + `)` + strings.ReplaceAll(SENTENCE_PATTERN, "{MAX_LENGTH}", fmt.Sprint(MAX_STANDALONE_LINE_LENGTH)) +
	`)`

var regex = regexp.MustCompile(pattern)

type RegexTextSplitter struct {
	Chunks []*DataPrepTextSplitterChunk
}

func NewRegexTextSplitter() (*RegexTextSplitter, error) {
	return &RegexTextSplitter{}, nil
}

func (r *RegexTextSplitter) AddDocument(filename, content, documentGroupID string) (string, error) {
	// Calculate the SHA256 hash of the content
	hash := sha256.Sum256([]byte(content))
	documentID := hex.EncodeToString(hash[:])[:10]

	// Split the content using the regex
	matches := regex.FindAllString(content, -1)

	// Create chunks from the matches
	for i, match := range matches {
		if match == "" {
			continue
		}

		r.Chunks = append(r.Chunks, &DataPrepTextSplitterChunk{
			Filename:        filename,
			Index:           i,
			Text:            match,
			DocumentID:      documentID,
			DocumentGroupID: documentGroupID,
		})
	}

	return documentID, nil
}
