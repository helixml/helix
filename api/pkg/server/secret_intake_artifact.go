package server

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"path"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	xhtml "golang.org/x/net/html"
)

const secretIntakeArtifactLimit = 32 * 1024
const secretIntakeFormMarker = "__HELIX_TRUSTED_SECRET_FORM__"

var intakeSafeTags = map[string]bool{"div": true, "section": true, "article": true, "header": true, "footer": true, "h1": true, "h2": true, "h3": true, "p": true, "strong": true, "em": true, "b": true, "i": true, "small": true, "span": true, "ul": true, "ol": true, "li": true, "br": true}
var intakeDiscardTags = map[string]bool{"script": true, "style": true, "iframe": true, "object": true, "embed": true, "svg": true, "math": true, "template": true, "form": true, "input": true, "button": true, "textarea": true, "select": true, "meta": true, "link": true}

// sanitizeSecretIntakeArtifact keeps static copy and one form slot. The artifact
// never controls the credential fields, form action, scripts, styles, or network.
func sanitizeSecretIntakeArtifact(raw string) (string, string, error) {
	doc, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	var body *xhtml.Node
	var findBody func(*xhtml.Node)
	findBody = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "body" {
			body = n
			return
		}
		for c := n.FirstChild; c != nil && body == nil; c = c.NextSibling {
			findBody(c)
		}
	}
	findBody(doc)
	if body == nil {
		return "", "", errors.New("artifact has no body")
	}
	var out strings.Builder
	markers := 0
	var render func(*xhtml.Node)
	render = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			out.WriteString(html.EscapeString(n.Data))
			return
		}
		if n.Type != xhtml.ElementNode {
			return
		}
		tag := strings.ToLower(n.Data)
		for _, a := range n.Attr {
			if a.Key == "data-helix-form" {
				if tag != "div" && tag != "form" {
					return
				}
				markers++
				out.WriteString(secretIntakeFormMarker)
				return
			}
		}
		if intakeDiscardTags[tag] {
			return
		}
		if intakeSafeTags[tag] {
			out.WriteString("<" + tag + ">")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			render(c)
		}
		if intakeSafeTags[tag] && tag != "br" {
			out.WriteString("</" + tag + ">")
		}
	}
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		render(c)
	}
	if markers != 1 {
		return "", "", errors.New("artifact must contain exactly one data-helix-form placeholder")
	}
	before, after, _ := strings.Cut(out.String(), secretIntakeFormMarker)
	return before, after, nil
}

func (s *HelixAPIServer) loadSecretIntakeArtifact(ctx context.Context, projectID, artifactID string) (string, string, error) {
	artifact, err := s.Store.GetArtifact(ctx, artifactID)
	if err != nil || artifact.ProjectID != projectID || artifact.Kind != types.ArtifactKindSingleFile || !strings.HasSuffix(strings.ToLower(artifact.Entrypoint), ".html") || artifact.ActiveVersion == nil || s.Controller == nil || s.Controller.Options.Filestore == nil {
		return "", "", errors.New("artifact_id must refer to a single-file HTML artifact in this project")
	}
	filename, meta, ok := resolveArtifactFile(artifact, "")
	if !ok || meta.Size > secretIntakeArtifactLimit {
		return "", "", errors.New("artifact HTML is unavailable or too large")
	}
	reader, err := s.Controller.Options.Filestore.OpenFile(ctx, path.Join(artifact.ActiveVersion.StoragePrefix, filename))
	if err != nil {
		return "", "", fmt.Errorf("open artifact HTML: %w", err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(io.LimitReader(reader, secretIntakeArtifactLimit+1))
	if err != nil || len(raw) > secretIntakeArtifactLimit {
		return "", "", errors.New("artifact HTML is unavailable or too large")
	}
	return sanitizeSecretIntakeArtifact(string(raw))
}
