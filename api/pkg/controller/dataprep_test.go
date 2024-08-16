package controller

import (
	"io"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
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
	suite.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil)

	// Call the getRagChunksToProcess function
	chunks, err := c.getRagChunksToProcess(session)
	suite.NoError(err)
	suite.NotNil(chunks)

	// Side effect -- Check that the documents have been added to the session
	suite.Equal(1, len(session.Metadata.DocumentIDs))
}
