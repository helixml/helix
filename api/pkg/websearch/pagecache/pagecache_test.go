package pagecache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
	"github.com/helixml/helix/api/pkg/websearch/native"
)

func TestPutStoresUnderNormalisedURLAndDocumentID(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	fetched := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)

	st.EXPECT().UpsertWebPage(gomock.Any(), &types.WebPage{
		ID:          websearch.DocumentID("https://example.com/a"),
		URL:         "https://example.com/a",
		FinalURL:    "https://www.example.com/a",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte("<html/>"),
		FetchedAt:   fetched,
	}).Return(nil)

	err := New(st).Put(context.Background(), &native.RawPage{
		URL:         "HTTPS://Example.com:443/a#frag",
		FinalURL:    "https://www.example.com/a",
		StatusCode:  200,
		ContentType: "text/html",
		Body:        []byte("<html/>"),
		FetchedAt:   fetched,
	})
	require.NoError(t, err)
}

func TestGetLooksUpNormalisedURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetWebPageByURL(gomock.Any(), "https://example.com/").Return(&types.WebPage{
		URL: "https://example.com/", StatusCode: 200, Body: []byte("x"),
	}, nil)

	page, err := New(st).Get(context.Background(), "https://EXAMPLE.com")
	require.NoError(t, err)
	require.Equal(t, &native.RawPage{URL: "https://example.com/", StatusCode: 200, Body: []byte("x")}, page)
}

func TestMissIsNilNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetWebPageByURL(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	st.EXPECT().GetWebPage(gomock.Any(), "doc_x").Return(nil, store.ErrNotFound)

	c := New(st)
	page, err := c.Get(context.Background(), "https://example.com/missing")
	require.NoError(t, err)
	require.Nil(t, page)

	page, err = c.GetByID(context.Background(), "doc_x")
	require.NoError(t, err)
	require.Nil(t, page)
}

func TestStoreErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	boom := errors.New("boom")
	st.EXPECT().GetWebPage(gomock.Any(), "doc_x").Return(nil, boom)

	_, err := New(st).GetByID(context.Background(), "doc_x")
	require.ErrorIs(t, err, boom)
}
