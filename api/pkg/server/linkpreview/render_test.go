package linkpreview

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestRenderPNG(t *testing.T) {
	label, c := statusStyle(types.TaskStatusImplementationReview)
	p := &Preview{TaskID: "spt_1", Number: 4294, Title: "Rich Slack/social link previews for spec tasks with a nice generated image card", StatusLabel: label, StatusColor: c, Project: "Helix", PRTotal: 3, PROpen: 1, PRMerged: 2}
	b, err := RenderPNG(p)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got.X != ImageWidth || got.Y != ImageHeight {
		t.Fatalf("size = %v", got)
	}
	if out := os.Getenv("LINKPREVIEW_SAMPLE_OUT"); out != "" {
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
