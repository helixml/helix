package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *ActionTestSuite) TestAction_runApiAction_showPetById() {
	called := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/pets/99944", r.URL.Path)
		suite.Equal("GET", r.Method)

		fmt.Fprintln(w, "{\"id\": 99944, \"name\": \"doggie\", \"tag\": \"dog\", \"description\": \"a brown dog\"}")

		called = true
	}))
	defer ts.Close()

	getPetDetailsAPI := &types.Tool{
		Name:        "getPetDetail",
		Description: "pet store API that is used to get details for the specified pet's ID",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolApiConfig{
				URL:    ts.URL,
				Schema: petStoreApiSpec,
				Actions: []*types.ToolApiAction{
					{
						Name:        "listPets",
						Description: "List all pets",
						Method:      "GET",
						Path:        "/pets",
					},
					{
						Name:        "createPets",
						Description: "Create a pet",
						Method:      "POST",
						Path:        "/pets",
					},
					{
						Name:        "showPetById",
						Description: "Info for a specific pet",
						Method:      "GET",
						Path:        "/pets/{petId}",
					},
				},
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "Can you please give me the details for pet 99944?"

	resp, err := suite.strategy.RunAction(suite.ctx, getPetDetailsAPI, history, currentMessage, "showPetById")
	suite.NoError(err)

	spew.Dump(resp)

	suite.True(called, "expected to call the API")
}
