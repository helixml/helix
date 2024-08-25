package text

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"text/template"
)

// Define constants (you may want to make these configurable)
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
	LOOKAHEAD_RANGE                    = 100
)

// ... (rest of the constants)

// Construct the regex pattern using text/template
var patternTemplate = `(?:^(?:[#*=-]{1,{{.MAX_HEADING_LENGTH}}|\w[^\r\n]{0,{{.MAX_HEADING_CONTENT_LENGTH}}}\r?\n[-=]{2,{{.MAX_HEADING_UNDERLINE_LENGTH}}}|<h[1-6][^>]{0,{{.MAX_HTML_HEADING_ATTRIBUTES_LENGTH}}>)[^\r\n]{1,{{.MAX_HEADING_CONTENT_LENGTH}}}(?:</h[1-6]>)?(?:\r?\n|$))|(?:\[[0-9]+\][^\r\n]{1,{{.MAX_STANDALONE_LINE_LENGTH}}})|(?:(?:^|\r?\n)[ \t]{0,3}(?:[-*+•]|\d{1,3}\.\w\.|\[[ xX]\])[ \t]+[^\r\n]{1,{{.MAX_LIST_ITEM_LENGTH}}})|(?:(?:^>(?:>|\s{2,}){0,2}[^\r\n]{1,{{.MAX_BLOCKQUOTE_LINE_LENGTH}}}\r?\n?){1,{{.MAX_BLOCKQUOTE_LINES}}})|(?:(?:^|\r?\n)(?:` + "```" + `|~~~)(?:\w{0,{{.MAX_CODE_LANGUAGE_LENGTH}}})?\r?\n[\s\S]{0,{{.MAX_CODE_BLOCK_LENGTH}}}?(?:` + "```" + `|~~~)\r?\n?)|(?:(?:^|\r?\n)(?:\|[^\r\n]{0,{{.MAX_TABLE_CELL_LENGTH}}}\|(?:\r?\n\|[-:]{1,{{.MAX_TABLE_CELL_LENGTH}}}\|){0,1}(?:\r?\n\|[^\r\n]{0,{{.MAX_TABLE_CELL_LENGTH}}}\|){0,{{.MAX_TABLE_ROWS}}}|<table>[\s\S]{0,{{.MAX_HTML_TABLE_LENGTH}}}?</table>))|(?:^(?:[-*_]){{{.MIN_HORIZONTAL_RULE_LENGTH}},}\s*$|<hr\s*/?>)|(?:[^\r\n]{1,{{.MAX_STANDALONE_LINE_LENGTH}}}(?:\r?\n|$))|(?:[^\r\n]{1,{{.MAX_SENTENCE_LENGTH}}})|(?:(?:^|\r?\n\r?\n)(?:<p>)?[^\r\n]{1,{{.MAX_PARAGRAPH_LENGTH}}}(?:</p>)?(?=\r?\n\r?\n|$))|(?:<[a-zA-Z][^>]{0,{{.MAX_HTML_TAG_ATTRIBUTES_LENGTH}}}(?:>[\s\S]{0,{{.MAX_HTML_TAG_CONTENT_LENGTH}}}?</[a-zA-Z]+>|\s*/>))|(?:(?:\$\$[\s\S]{0,{{.MAX_MATH_BLOCK_LENGTH}}}?\$\$)|(?:\$[^\$\r\n]{0,{{.MAX_MATH_INLINE_LENGTH}}}\$))`

var pattern string

func init() {
	tmpl, err := template.New("pattern").Parse(patternTemplate)
	if err != nil {
		panic(err)
	}

	data := struct {
		MAX_HEADING_LENGTH                 int
		MAX_HEADING_CONTENT_LENGTH         int
		MAX_HEADING_UNDERLINE_LENGTH       int
		MAX_HTML_HEADING_ATTRIBUTES_LENGTH int
		MAX_LIST_ITEM_LENGTH               int
		MAX_BLOCKQUOTE_LINE_LENGTH         int
		MAX_BLOCKQUOTE_LINES               int
		MAX_CODE_LANGUAGE_LENGTH           int
		MAX_CODE_BLOCK_LENGTH              int
		MAX_TABLE_CELL_LENGTH              int
		MAX_TABLE_ROWS                     int
		MAX_HTML_TABLE_LENGTH              int
		MIN_HORIZONTAL_RULE_LENGTH         int
		MAX_STANDALONE_LINE_LENGTH         int
		MAX_SENTENCE_LENGTH                int
		MAX_PARAGRAPH_LENGTH               int
		MAX_HTML_TAG_ATTRIBUTES_LENGTH     int
		MAX_HTML_TAG_CONTENT_LENGTH        int
		MAX_MATH_BLOCK_LENGTH              int
		MAX_MATH_INLINE_LENGTH             int
	}{
		MAX_HEADING_LENGTH,
		MAX_HEADING_CONTENT_LENGTH,
		MAX_HEADING_UNDERLINE_LENGTH,
		MAX_HTML_HEADING_ATTRIBUTES_LENGTH,
		MAX_LIST_ITEM_LENGTH,
		MAX_BLOCKQUOTE_LINE_LENGTH,
		MAX_BLOCKQUOTE_LINES,
		MAX_CODE_LANGUAGE_LENGTH,
		MAX_CODE_BLOCK_LENGTH,
		MAX_TABLE_CELL_LENGTH,
		MAX_TABLE_ROWS,
		MAX_HTML_TABLE_LENGTH,
		MIN_HORIZONTAL_RULE_LENGTH,
		MAX_STANDALONE_LINE_LENGTH,
		MAX_SENTENCE_LENGTH,
		MAX_PARAGRAPH_LENGTH,
		MAX_HTML_TAG_ATTRIBUTES_LENGTH,
		MAX_HTML_TAG_CONTENT_LENGTH,
		MAX_MATH_BLOCK_LENGTH,
		MAX_MATH_INLINE_LENGTH,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(err)
	}

	pattern = buf.String()
}

var regex = regexp.MustCompile(pattern)

// regex, err := regexp.Compile(pattern)
// if err != nil {
// 	return nil, fmt.Errorf("failed to compile regex: %w", err)
// }

type RegexTextSplitter struct {
	Chunks []*DataPrepTextSplitterChunk
	regex  *regexp.Regexp
}

func NewRegexTextSplitter() (*RegexTextSplitter, error) {

	return &RegexTextSplitter{
		// regex: regex,
	}, nil
}

func (r *RegexTextSplitter) AddDocument(filename, content, documentGroupID string) (string, error) {
	// Calculate the SHA256 hash of the content
	hash := sha256.Sum256([]byte(content))
	documentID := hex.EncodeToString(hash[:])[:10]

	// Split the content using the regex
	matches := regex.FindAllString(content, -1)

	// Create chunks from the matches
	for i, match := range matches {
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
