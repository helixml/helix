package services

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestBuildAttachmentsSection_Empty(t *testing.T) {
	// No attachments → no section, no whitespace pollution into the prompt.
	assert.Equal(t, "", BuildAttachmentsSection(nil, "2026-05-19_dark-mode_1"))
	assert.Equal(t, "", BuildAttachmentsSection([]*types.SpecTaskAttachment{}, "x"))
}

func TestBuildAttachmentsSection_Renders(t *testing.T) {
	attachments := []*types.SpecTaskAttachment{
		{ID: "att_1", Filename: "screenshot.png", MimeType: "image/png", SizeBytes: 248 * 1024, Caption: "shows misaligned dropdown"},
		{ID: "att_2", Filename: "design.pdf", MimeType: "application/pdf", SizeBytes: 1_300_000},
	}
	out := BuildAttachmentsSection(attachments, "2026-05-19_dark-mode_1")

	// Header is set with file count.
	assert.True(t, strings.HasPrefix(out, "## Attachments\n"))
	assert.Contains(t, out, "attached 2 file(s)")

	// Each file shows the canonical workspace path the agent should read from.
	assert.Contains(t, out, "/home/retro/work/helix-specs/design/tasks/2026-05-19_dark-mode_1/attachments/screenshot.png")
	assert.Contains(t, out, "/home/retro/work/helix-specs/design/tasks/2026-05-19_dark-mode_1/attachments/design.pdf")

	// Mime type + size are formatted.
	assert.Contains(t, out, "image/png, 248 KB")
	assert.Contains(t, out, "application/pdf, 1.2 MB")

	// Caption is rendered for the one that has it; quoted to make it clear it's text.
	assert.Contains(t, out, `"shows misaligned dropdown"`)

	// Closing instruction is present (this is the only thing telling the agent to
	// actually open the files before guessing).
	assert.Contains(t, out, "Read or view them BEFORE asking clarifying questions")
}

func TestHumanSize(t *testing.T) {
	assert.Equal(t, "999 B", humanSize(999))
	assert.Equal(t, "1 KB", humanSize(1024))
	assert.Equal(t, "248 KB", humanSize(248*1024))
	assert.Equal(t, "1.2 MB", humanSize(1_258_291)) // ~1.2 MB
}
