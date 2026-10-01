package spectask

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/helixml/helix/api/pkg/types"
)

var attachFlagLimits = fmt.Sprintf(
	"Accepts images, PDF, text/markdown/CSV, and gzip/zip archives (compress large logs); max %s per file and %s per task creation.",
	humanize.IBytes(uint64(types.SpecTaskAttachmentMaxBytes)),
	humanize.IBytes(uint64(types.SpecTaskInlineAttachmentsMaxBytes)),
)

// readInlineAttachments loads --attach files for submission in the same request
// that creates the task. The server validates every file before creating
// anything, so one bad file means no task is created and every problem is
// reported together. Local problems (unreadable or oversized files) are caught
// here, also all at once, before anything is sent.
func readInlineAttachments(paths []string) ([]types.SpecTaskInlineAttachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	var problems []string
	var total int64
	for _, path := range paths {
		info, err := os.Stat(path)
		switch {
		case err != nil:
			problems = append(problems, err.Error())
		case info.IsDir():
			problems = append(problems, fmt.Sprintf("%s is a directory", path))
		case info.Size() > types.SpecTaskAttachmentMaxBytes:
			problems = append(problems, fmt.Sprintf("%s is %s, over the %s per-file limit",
				path, humanize.IBytes(uint64(info.Size())), humanize.IBytes(uint64(types.SpecTaskAttachmentMaxBytes))))
		default:
			total += info.Size()
		}
	}
	if total > types.SpecTaskInlineAttachmentsMaxBytes {
		problems = append(problems, fmt.Sprintf("attachments total %s, over the %s limit per task creation",
			humanize.IBytes(uint64(total)), humanize.IBytes(uint64(types.SpecTaskInlineAttachmentsMaxBytes))))
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("--attach rejected; nothing was created:\n- %s", strings.Join(problems, "\n- "))
	}

	attachments := make([]types.SpecTaskInlineAttachment, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read --attach %q: %w", path, err)
		}
		attachments = append(attachments, types.SpecTaskInlineAttachment{
			Name:          filepath.Base(path),
			ContentBase64: base64.StdEncoding.EncodeToString(data),
		})
	}
	return attachments, nil
}
