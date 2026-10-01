package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// chatAttachmentManifestHeader matches CHAT_ATTACHMENT_MANIFEST_HEADER in
// frontend/src/components/common/chatAttachments.ts, so the chat renders
// attachments sent through the API the same as ones uploaded in the UI.
const chatAttachmentManifestHeader = "Attachments available in the agent workspace:"

// chatAttachment is an inline image or file part of a chat message.
type chatAttachment struct {
	image    bool
	filename string
	data     []byte
}

func validateNewBotChatRequest(req *types.SessionChatRequest) error {
	if len(req.Messages) != 1 {
		return fmt.Errorf("a new bot chat requires exactly one message")
	}
	_, _, err := splitChatAttachments(req.MessageContent())
	return err
}

// splitChatAttachments separates a message into its text and its inline
// attachments. Attachments must be data: URLs: image_url parts, or OpenAI
// file parts ({"type":"file","file":{"filename","file_data"}}). Helix never
// fetches a remote URL on a caller's behalf.
func splitChatAttachments(content types.MessageContent) (string, []chatAttachment, error) {
	var texts []string
	var attachments []chatAttachment
	for _, part := range content.Parts {
		switch p := part.(type) {
		case nil:
		case string:
			texts = append(texts, p)
		case types.TextPart:
			texts = append(texts, p.Text)
		case types.ImageURLPart:
			attachment, err := decodeChatAttachment(p.ImageURL.URL, "", true, len(attachments)+1)
			if err != nil {
				return "", nil, err
			}
			attachments = append(attachments, attachment)
		case map[string]any:
			switch p["type"] {
			case "text":
				text, _ := p["text"].(string)
				texts = append(texts, text)
			case "image_url":
				image, _ := p["image_url"].(map[string]any)
				dataURL, _ := image["url"].(string)
				attachment, err := decodeChatAttachment(dataURL, "", true, len(attachments)+1)
				if err != nil {
					return "", nil, err
				}
				attachments = append(attachments, attachment)
			case "file":
				file, _ := p["file"].(map[string]any)
				dataURL, _ := file["file_data"].(string)
				filename, _ := file["filename"].(string)
				attachment, err := decodeChatAttachment(dataURL, filename, false, len(attachments)+1)
				if err != nil {
					return "", nil, err
				}
				attachments = append(attachments, attachment)
			default:
				return "", nil, fmt.Errorf("unsupported message part type %v", p["type"])
			}
		default:
			return "", nil, fmt.Errorf("unsupported message part %T", part)
		}
	}
	return strings.Join(texts, "\n\n"), attachments, nil
}

func decodeChatAttachment(dataURL, filename string, image bool, index int) (chatAttachment, error) {
	header, encoded, ok := strings.Cut(strings.TrimPrefix(dataURL, "data:"), ",")
	if !ok || !strings.HasPrefix(dataURL, "data:") || !strings.HasSuffix(header, ";base64") {
		return chatAttachment{}, fmt.Errorf("attachment %d must be a base64 data: URL", index)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return chatAttachment{}, fmt.Errorf("attachment %d: invalid base64: %w", index, err)
	}
	filename = filepath.Base(filename)
	if filename == "." || filename == "/" {
		filename = ""
	}
	if filename == "" {
		ext := ".bin"
		if exts, _ := mime.ExtensionsByType(strings.TrimSuffix(header, ";base64")); len(exts) > 0 {
			ext = exts[0]
		}
		kind := "file"
		if image {
			kind = "image"
		}
		filename = fmt.Sprintf("%s-%d%s", kind, index, ext)
	}
	return chatAttachment{image: image, filename: filename, data: data}, nil
}

// moveChatAttachmentsToWorkspace writes the inline attachments of the
// request's last message into the session's ~/work/incoming/ and replaces
// them with the manifest the chat composer sends after an upload. Coding
// agents read files from the workspace; an image part would otherwise be
// dropped without a word.
func (s *HelixAPIServer) moveChatAttachmentsToWorkspace(ctx context.Context, user *types.User, session *types.Session, req *types.SessionChatRequest) *system.HTTPError {
	last := req.Messages[len(req.Messages)-1]
	text, attachments, err := splitChatAttachments(last.Content)
	if err != nil {
		return system.NewHTTPError400(err.Error())
	}
	if len(attachments) == 0 {
		return nil
	}

	manifest := []string{chatAttachmentManifestHeader}
	for _, attachment := range attachments {
		path, httpErr := s.uploadChatAttachment(ctx, user, session, attachment)
		if httpErr != nil {
			return httpErr
		}
		kind := "File"
		if attachment.image {
			kind = "Image"
		}
		quoted, _ := json.Marshal(path)
		manifest = append(manifest, fmt.Sprintf("- %s: %s", kind, quoted))
	}
	block := strings.Join(manifest, "\n")
	if text != "" {
		block = text + "\n\n" + block
	}
	last.Content = types.MessageContent{ContentType: types.MessageContentTypeText, Parts: []any{block}}
	return nil
}

func (s *HelixAPIServer) uploadChatAttachment(ctx context.Context, user *types.User, session *types.Session, attachment chatAttachment) (string, *system.HTTPError) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", attachment.filename)
	if err != nil {
		return "", system.NewHTTPError500("encode attachment: " + err.Error())
	}
	if _, err := part.Write(attachment.data); err != nil {
		return "", system.NewHTTPError500("encode attachment: " + err.Error())
	}
	if err := form.Close(); err != nil {
		return "", system.NewHTTPError500("encode attachment: " + err.Error())
	}
	respBody, httpErr := s.uploadToSandbox(ctx, user, session, &body, form.FormDataContentType(), int64(body.Len()), "open_file_manager=false")
	if httpErr != nil {
		return "", httpErr
	}
	var uploaded types.SandboxFileUploadResponse
	if err := json.Unmarshal(respBody, &uploaded); err != nil || uploaded.Path == "" {
		return "", system.NewHTTPError500("sandbox upload returned no path")
	}
	return uploaded.Path, nil
}
