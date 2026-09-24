package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Parts as a gateway sends them: decoded from JSON into maps.
func jsonParts(t *testing.T, raw string) types.MessageContent {
	var content types.MessageContent
	require.NoError(t, json.Unmarshal([]byte(raw), &content))
	return content
}

func TestSplitChatAttachmentsDecodesImagesAndFiles(t *testing.T) {
	content := jsonParts(t, `{"content_type":"multimodal_text","parts":[
		{"type":"text","text":"Is this document valid?"},
		{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}},
		{"type":"file","file":{"filename":"../../etc/passport.pdf","file_data":"data:application/pdf;base64,cGRm"}}
	]}`)

	text, attachments, err := splitChatAttachments(content)
	require.NoError(t, err)
	require.Equal(t, "Is this document valid?", text)
	require.Len(t, attachments, 2)

	require.True(t, attachments[0].image)
	require.Equal(t, "image-1.png", attachments[0].filename)
	require.Equal(t, "hello", string(attachments[0].data))

	require.False(t, attachments[1].image)
	require.Equal(t, "passport.pdf", attachments[1].filename, "a path in the filename must not escape incoming/")
	require.Equal(t, "pdf", string(attachments[1].data))
}

func TestSplitChatAttachmentsLeavesPlainTextAlone(t *testing.T) {
	text, attachments, err := splitChatAttachments(jsonParts(t, `{"content_type":"text","parts":["hi"]}`))
	require.NoError(t, err)
	require.Equal(t, "hi", text)
	require.Empty(t, attachments)
}

// Helix never fetches a URL for a caller, and never drops a part silently.
func TestSplitChatAttachmentsRejectsWhatItCannotDeliver(t *testing.T) {
	for _, raw := range []string{
		`{"parts":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}`,
		`{"parts":[{"type":"image_url","image_url":{"url":"data:image/png,notbase64"}}]}`,
		`{"parts":[{"type":"file","file":{"filename":"a.pdf","file_data":"data:application/pdf;base64,!!!"}}]}`,
		`{"parts":[{"type":"input_audio","input_audio":{"data":"AAAA"}}]}`,
	} {
		_, _, err := splitChatAttachments(jsonParts(t, raw))
		require.Error(t, err, raw)
	}
}

func TestBotInstanceTurnWebhookOnlyForInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	interaction := &types.Interaction{ID: "int_1", State: types.InteractionStateComplete, ResponseMessage: "Valid until 2031."}

	instance := instanceSession()
	instance.ProjectID = "prj_bot"
	instance.ParentApp = "app_bot"
	st.EXPECT().EnqueueWebhookEvent(gomock.Any(), types.WebhookEventBotInstanceTurnCompleted, "org_one", "prj_bot", types.BotInstanceTurnWebhookData{
		SessionID:      "ses_instance",
		InteractionID:  "int_1",
		BotID:          "b-broker",
		AppID:          "app_bot",
		ProjectID:      "prj_bot",
		OrganizationID: "org_one",
		State:          types.InteractionStateComplete,
		Response:       "Valid until 2031.",
	}).Return(nil)
	server.enqueueBotInstanceTurnWebhook(context.Background(), instance, interaction)

	mainSession := instanceSession()
	mainSession.Metadata.SessionRole = "exploratory"
	server.enqueueBotInstanceTurnWebhook(context.Background(), mainSession, interaction)
}
