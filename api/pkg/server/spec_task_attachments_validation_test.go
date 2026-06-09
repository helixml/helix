package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestSanitiseAttachmentFilename(t *testing.T) {
	cases := []struct {
		name, in, out string
	}{
		{"basic", "screenshot.png", "screenshot.png"},
		{"strips parent dir", "/etc/passwd", "passwd"},
		{"strips relative", "../../../../etc/shadow", "shadow"},
		{"trims whitespace", "  notes.md  ", "notes.md"},
		{"hidden file rejected", ".env", ""},
		{"empty rejected", "", ""},
		{"current dir rejected", ".", ""},
		{"parent dir rejected", "..", ""},
		{"backslash rejected", "a\\b.png", ""},
		{"null byte rejected", "a\x00b.png", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.out, sanitiseAttachmentFilename(tc.in))
		})
	}
}

func TestDetectAttachmentMime_ExtensionWins(t *testing.T) {
	// Markdown — sniffer returns text/plain, extension upgrades to text/markdown.
	assert.Equal(t, "text/markdown", detectAttachmentMime("notes.md", []byte("# hello")))
	// SVG — sniffer returns text/xml, extension fixes to image/svg+xml.
	assert.Equal(t, "image/svg+xml", detectAttachmentMime("logo.svg", []byte("<svg></svg>")))
	// CSV — sniffer returns text/plain, extension fixes to text/csv.
	assert.Equal(t, "text/csv", detectAttachmentMime("data.csv", []byte("a,b\n1,2\n")))
}

func TestDetectAttachmentMime_SnifferWinsWhenExtUnknown(t *testing.T) {
	// PNG magic bytes.
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	assert.Equal(t, "image/png", detectAttachmentMime("screenshot", png))
	// PDF magic bytes.
	pdf := append([]byte("%PDF-1.4\n"), make([]byte, 64)...)
	assert.Equal(t, "application/pdf", detectAttachmentMime("doc", pdf))
}

func TestSvgContainsScript(t *testing.T) {
	clean := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`)
	bad := []byte(`<svg><script>alert(1)</script></svg>`)
	assert.False(t, svgContainsScript(clean))
	assert.True(t, svgContainsScript(bad))
	// Case-insensitive
	assert.True(t, svgContainsScript([]byte(`<SVG><SCRIPT/></SVG>`)))
}

func TestSpecTaskAttachmentsLocked(t *testing.T) {
	for _, status := range []types.SpecTaskStatus{
		types.TaskStatusBacklog,
		types.TaskStatusQueuedSpecGeneration,
		types.TaskStatusSpecGeneration,
		types.TaskStatusSpecReview,
		types.TaskStatusSpecRevision,
	} {
		assert.False(t, specTaskAttachmentsLocked(status), "expected unlocked for %s", status)
	}
	for _, status := range []types.SpecTaskStatus{
		types.TaskStatusSpecApproved,
		types.TaskStatusImplementationQueued,
		types.TaskStatusImplementation,
		types.TaskStatusImplementationReview,
		types.TaskStatusPullRequest,
		types.TaskStatusDone,
		types.TaskStatusImplementationFailed,
	} {
		assert.True(t, specTaskAttachmentsLocked(status), "expected locked for %s", status)
	}
}

func TestAllowedMimeTypes(t *testing.T) {
	for _, mime := range []string{
		"image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml",
		"application/pdf", "text/plain", "text/markdown", "text/csv",
	} {
		assert.True(t, types.SpecTaskAttachmentAllowedMimeTypes[mime], "%s should be allowed", mime)
	}
	for _, mime := range []string{
		"image/bmp", "application/zip", "application/x-executable",
		"video/mp4", "text/html",
	} {
		assert.False(t, types.SpecTaskAttachmentAllowedMimeTypes[mime], "%s should be rejected", mime)
	}
}
