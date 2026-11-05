package desktop

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOwner answers SelectionRead the way a real owner does: only for the
// types it holds data for. Every requested type is recorded.
type fakeOwner struct {
	data      map[string][]byte
	requested []string
}

func (o *fakeOwner) read(mimeType string) ([]byte, error) {
	o.requested = append(o.requested, mimeType)
	return o.data[mimeType], nil
}

func encodeTestImage(t *testing.T, encode func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for x := 0; x < 8; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: 30, G: 140, B: 250, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, encode(&buf, img))
	return buf.Bytes()
}

func testPNG(t *testing.T) []byte {
	return encodeTestImage(t, func(b *bytes.Buffer, img image.Image) error { return png.Encode(b, img) })
}

func decodeImageData(t *testing.T, data *ClipboardData) image.Image {
	t.Helper()
	require.Equal(t, "image", data.Type)
	raw, err := base64.StdEncoding.DecodeString(data.Data)
	require.NoError(t, err)
	img, format, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	assert.Equal(t, "png", format)
	return img
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func TestReadClipboardOffer_ImageOnly(t *testing.T) {
	pngBytes := testPNG(t)
	owner := &fakeOwner{data: map[string][]byte{"image/png": pngBytes}}

	got, err := readClipboardOffer([]string{"image/png"}, owner.read)
	require.NoError(t, err)

	assert.Equal(t, "image", got.Type)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pngBytes), got.Data)
	// The bug this fixes: text was requested first and an owner that answers
	// every type (wl-copy) handed back the PNG bytes as "text".
	assert.Equal(t, []string{"image/png"}, owner.requested)
}

func TestReadClipboardOffer_ImageWinsOverAccompanyingText(t *testing.T) {
	// Firefox "Copy Image" advertises the image URL as text alongside the image.
	pngBytes := testPNG(t)
	owner := &fakeOwner{data: map[string][]byte{
		"image/png":  pngBytes,
		"text/plain": []byte("https://example.com/cat.png"),
	}}

	got, err := readClipboardOffer([]string{"text/html", "text/plain", "image/png"}, owner.read)
	require.NoError(t, err)
	assert.Equal(t, "image", got.Type)
}

func TestReadClipboardOffer_TextOnlyNeverRequestsImage(t *testing.T) {
	owner := &fakeOwner{data: map[string][]byte{
		"text/plain;charset=utf-8": []byte("hello from the desktop"),
		// An owner that would answer an unadvertised image request.
		"image/png": testPNG(t),
	}}

	got, err := readClipboardOffer([]string{"text/plain;charset=utf-8", "UTF8_STRING"}, owner.read)
	require.NoError(t, err)

	assert.Equal(t, &ClipboardData{Type: "text", Data: "hello from the desktop"}, got)
	assert.Equal(t, []string{"text/plain;charset=utf-8"}, owner.requested)
}

func TestReadClipboardOffer_FallsBackThroughAdvertisedTextTypes(t *testing.T) {
	owner := &fakeOwner{data: map[string][]byte{"UTF8_STRING": []byte("from an X11 app")}}

	got, err := readClipboardOffer([]string{"UTF8_STRING", "STRING"}, owner.read)
	require.NoError(t, err)
	assert.Equal(t, &ClipboardData{Type: "text", Data: "from an X11 app"}, got)
}

func TestReadClipboardOffer_NoOwnerIsEmpty(t *testing.T) {
	owner := &fakeOwner{}

	got, err := readClipboardOffer(nil, owner.read)
	require.NoError(t, err)
	assert.Equal(t, &ClipboardData{Type: "text", Data: ""}, got)
	assert.Empty(t, owner.requested)
}

func TestReadClipboardOffer_ImageFileCopiedInLoupe(t *testing.T) {
	// Loupe (and Files) copy a file reference, not pixels: exactly these types.
	path := filepath.Join(t.TempDir(), "photo.png")
	pngBytes := testPNG(t)
	require.NoError(t, os.WriteFile(path, pngBytes, 0o644))
	owner := &fakeOwner{data: map[string][]byte{
		"text/uri-list":            []byte(fileURI(path) + "\r\n"),
		"text/plain;charset=utf-8": []byte(path),
	}}

	got, err := readClipboardOffer([]string{
		"application/vnd.portal.files", "application/vnd.portal.filetransfer",
		"text/uri-list", "text/plain;charset=utf-8",
	}, owner.read)
	require.NoError(t, err)

	assert.Equal(t, "image", got.Type)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pngBytes), got.Data)
}

func TestReadClipboardOffer_JPEGFileIsTranscodedToPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo with spaces.jpg")
	jpegBytes := encodeTestImage(t, func(b *bytes.Buffer, img image.Image) error { return jpeg.Encode(b, img, nil) })
	require.NoError(t, os.WriteFile(path, jpegBytes, 0o644))
	owner := &fakeOwner{data: map[string][]byte{"text/uri-list": []byte(fileURI(path))}}

	got, err := readClipboardOffer([]string{"text/uri-list"}, owner.read)
	require.NoError(t, err)

	img := decodeImageData(t, got)
	assert.Equal(t, image.Rect(0, 0, 8, 4), img.Bounds())
}

func TestReadClipboardOffer_NonImageFileStaysText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("not an image"), 0o644))
	owner := &fakeOwner{data: map[string][]byte{
		"text/uri-list":            []byte(fileURI(path)),
		"text/plain;charset=utf-8": []byte(path),
	}}

	got, err := readClipboardOffer([]string{"text/uri-list", "text/plain;charset=utf-8"}, owner.read)
	require.NoError(t, err)
	assert.Equal(t, &ClipboardData{Type: "text", Data: path}, got)
}

func TestImageFileFromURIList_RejectsAnythingButOneLocalImage(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.png")
	require.NoError(t, os.WriteFile(a, testPNG(t), 0o644))
	require.NoError(t, os.WriteFile(b, testPNG(t), 0o644))

	_, ok := imageFileFromURIList([]byte(fileURI(a) + "\n" + fileURI(b) + "\n"))
	assert.False(t, ok, "multiple files")

	_, ok = imageFileFromURIList([]byte("https://example.com/cat.png"))
	assert.False(t, ok, "remote URL")

	_, ok = imageFileFromURIList([]byte(fileURI(filepath.Join(dir, "missing.png"))))
	assert.False(t, ok, "missing file")

	_, ok = imageFileFromURIList([]byte(fileURI(dir)))
	assert.False(t, ok, "directory")

	got, ok := imageFileFromURIList([]byte("# comment\n" + fileURI(a) + "\n"))
	assert.True(t, ok, "comment lines are ignored")
	assert.NotEmpty(t, got)
}

// selectionOwnerChangedBody round-trips a SelectionOwnerChanged signal through
// the D-Bus wire format, so the test sees the body exactly as godbus decodes it.
func selectionOwnerChangedBody(t *testing.T, options map[string]dbus.Variant) []interface{} {
	t.Helper()
	msg := &dbus.Message{
		Type: dbus.TypeSignal,
		Headers: map[dbus.HeaderField]dbus.Variant{
			dbus.FieldPath:      dbus.MakeVariant(dbus.ObjectPath("/org/gnome/Mutter/RemoteDesktop/Session/u1")),
			dbus.FieldInterface: dbus.MakeVariant(remoteDesktopSessionIface),
			dbus.FieldMember:    dbus.MakeVariant("SelectionOwnerChanged"),
			dbus.FieldSignature: dbus.MakeVariant(dbus.SignatureOf(options)),
		},
		Body: []interface{}{options},
	}
	var buf bytes.Buffer
	require.NoError(t, msg.EncodeTo(&buf, binary.LittleEndian))
	decoded, err := dbus.DecodeMessage(&buf)
	require.NoError(t, err)
	return decoded.Body
}

func TestParseSelectionOwnerChanged_MutterWireFormat(t *testing.T) {
	// As captured with dbus-monitor from Mutter after a Ctrl+C in Loupe:
	// mime-types is a variant holding the struct (as).
	loupe := []string{"application/vnd.portal.files", "application/vnd.portal.filetransfer", "text/uri-list", "text/plain;charset=utf-8"}
	body := selectionOwnerChangedBody(t, map[string]dbus.Variant{
		"mime-types":       dbus.MakeVariant(struct{ MimeTypes []string }{loupe}),
		"session-is-owner": dbus.MakeVariant(false),
	})
	require.Equal(t, "(as)", body[0].(map[string]dbus.Variant)["mime-types"].Signature().String())

	mimeTypes, sessionOwned, err := parseSelectionOwnerChanged(body)
	require.NoError(t, err)
	assert.Equal(t, loupe, mimeTypes)
	assert.False(t, sessionOwned)
}

func TestParseSelectionOwnerChanged_SessionOwner(t *testing.T) {
	body := selectionOwnerChangedBody(t, map[string]dbus.Variant{
		"mime-types":       dbus.MakeVariant(struct{ MimeTypes []string }{[]string{"image/png"}}),
		"session-is-owner": dbus.MakeVariant(true),
	})

	mimeTypes, sessionOwned, err := parseSelectionOwnerChanged(body)
	require.NoError(t, err)
	assert.Equal(t, []string{"image/png"}, mimeTypes)
	assert.True(t, sessionOwned)
}

func TestParseSelectionOwnerChanged_ClearedSelection(t *testing.T) {
	// Mutter sends empty options when the selection is cleared.
	mimeTypes, sessionOwned, err := parseSelectionOwnerChanged(selectionOwnerChangedBody(t, map[string]dbus.Variant{}))
	require.NoError(t, err)
	assert.Empty(t, mimeTypes)
	assert.False(t, sessionOwned)
}
