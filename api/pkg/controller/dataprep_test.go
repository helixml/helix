package controller

import (
	"io"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type DataPrepTestSuite struct {
	suite.Suite
	store     *store.MockStore
	filestore *filestore.MockFileStore
}

func TestDataPrepSuite(t *testing.T) {
	suite.Run(t, new(DataPrepTestSuite))
}

func (suite *DataPrepTestSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.store = store.NewMockStore(ctrl)
	suite.filestore = filestore.NewMockFileStore(ctrl)
}

func (suite *DataPrepTestSuite) TestGetRagChunksToProcess() {
	c := &Controller{}
	c.Options.Store = suite.store
	c.Options.Filestore = suite.filestore
	// PubSub is intentionally nil since publishEvent now handles this gracefully

	// Create a dummy session
	session := &types.Session{
		ID: "test-session",
		Interactions: types.Interactions{
			{
				Creator: types.CreatorTypeUser,
				Files: []string{
					"test-file.txt",
				},
			},
		},
		Metadata: types.SessionMetadata{
			RagSettings: types.RAGSettings{
				ChunkSize:     2,
				ChunkOverflow: 1,
			},
		},
	}

	// Mock the calls made by the getRagChunksToProcess function
	reader := io.NopCloser(strings.NewReader("test file content"))
	suite.filestore.EXPECT().OpenFile(gomock.Any(), "test-file.txt").Return(reader, nil)
	// First UpdateSession call happens in UpdateSessionMetadata
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil)
	// GetSession call from WriteSession
	suite.store.EXPECT().GetSession(gomock.Any(), "test-session").Return(session, nil)
	// Second UpdateSession call happens in WriteSession
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil)
	// No need to mock PubSub.Publish as publishEvent now handles nil PubSub

	// Call the getRagChunksToProcess function
	chunks, err := c.getRagChunksToProcess(session)
	suite.NoError(err)
	suite.NotNil(chunks)

	// Side effect -- Check that the documents have been added to the session
	suite.Equal(1, len(session.Metadata.DocumentIDs))
}
