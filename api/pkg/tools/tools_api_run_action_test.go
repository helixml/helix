package tools

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"

	"github.com/davecgh/go-spew/spew"
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
			API: &types.ToolAPIConfig{
				URL:    ts.URL,
				Schema: petStoreAPISpec,
				Actions: []*types.ToolAPIAction{
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

	history := []*types.ToolHistoryMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Can you please give me the details for pet 99944?",
		},
	}

	resp, err := suite.strategy.RunAction(suite.ctx, "session-123", "i-123", getPetDetailsAPI, history, "showPetById")
	suite.NoError(err)

	suite.strategy.wg.Wait()

	spew.Dump(resp)

	suite.True(called, "expected to call the API")

	fmt.Println("U:", history[0].Content)
	fmt.Println("A:", resp.Message)
}

func (suite *ActionTestSuite) TestAction_RunAPIActionWithParameters_createPet() {
	called := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/pets", r.URL.Path)
		suite.Equal("POST", r.Method)

		body, err := io.ReadAll(r.Body)
		suite.NoError(err)
		defer r.Body.Close()

		assert.Contains(suite.T(), string(body), "fluffy")
		assert.Contains(suite.T(), string(body), "dog")

		fmt.Fprintln(w, "{\"id\": 99944, \"name\": \"fluffy\", \"tag\": \"dog\", \"description\": \"a brown dog\"}")

		called = true
	}))
	defer ts.Close()

	managePetsAPI := &types.Tool{
		Name:        "managePetsApi",
		Description: "pet store API that is used to manage pets",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    ts.URL,
				Schema: petStoreAPISpec,
				Actions: []*types.ToolAPIAction{
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

	resp, err := suite.strategy.RunAPIActionWithParameters(suite.ctx, &types.RunAPIActionRequest{
		Tool:   managePetsAPI,
		Action: "createPets",
		Parameters: map[string]interface{}{
			"name": "fluffy",
			"tag":  "dog",
		},
	})
	suite.NoError(err)
	suite.strategy.wg.Wait()

	suite.True(called, "expected to call the API")

	fmt.Println("A:", resp.Response)
}

const weatherResp = `{
  "coord": { "lon": -0.1257, "lat": 51.5085 },
  "weather": [
    {
      "id": 803,
      "main": "Clouds",
      "description": "broken clouds",
      "icon": "04d"
    }
  ],
  "base": "stations",
  "main": {
    "temp": 282.28,
    "feels_like": 278.77,
    "temp_min": 281.1,
    "temp_max": 283.42,
    "pressure": 1021,
    "humidity": 83
  },
  "visibility": 10000,
  "wind": { "speed": 7.72, "deg": 240 },
  "clouds": { "all": 75 },
  "dt": 1707123392,
  "sys": {
    "type": 2,
    "id": 2075535,
    "country": "GB",
    "sunrise": 1707118416,
    "sunset": 1707152118
  },
  "timezone": 0,
  "id": 2643743,
  "name": "London",
  "cod": 200
}
`

func (suite *ActionTestSuite) TestAction_runApiAction_getWeather() {
	called := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/weather", r.URL.Path)

		suite.Equal("secret-key", r.URL.Query().Get("appid"))
		suite.Equal("London", r.URL.Query().Get("q"))
		suite.Equal("GET", r.Method)

		fmt.Fprint(w, weatherResp)

		called = true
	}))
	defer ts.Close()

	weatherSpec, err := os.ReadFile("./testdata/weather.yaml")
	suite.NoError(err)

	getPetDetailsAPI := &types.Tool{
		Name:        "getWeather",
		Description: "Weather API service that can be used to retrieve weather information for the given location",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    ts.URL,
				Schema: string(weatherSpec),
				Query: map[string]string{
					"appid": "secret-key",
				},
				Actions: []*types.ToolAPIAction{
					{
						Name:        "CurrentWeatherData",
						Description: "Call current weather data for one location",
						Method:      "GET",
						Path:        "/weather",
					},
				},
			},
		},
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What's the weather like in London?",
		},
	}

	resp, err := suite.strategy.RunAction(suite.ctx, "session-123", "i-123", getPetDetailsAPI, history, "CurrentWeatherData")
	suite.NoError(err)

	suite.strategy.wg.Wait()

	spew.Dump(resp)

	suite.True(called, "expected to call the API")

	fmt.Println("U:", history[0].Content)
	fmt.Println("A:", resp.Message)
}

func (suite *ActionTestSuite) TestAction_runApiAction_history_getWeather() {
	called := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/weather", r.URL.Path)

		suite.Equal("secret-key", r.URL.Query().Get("appid"))
		suite.Contains(strings.ToLower(r.URL.Query().Get("q")), "london")
		suite.Equal("GET", r.Method)

		fmt.Fprint(w, weatherResp)

		called = true
	}))
	defer ts.Close()

	// suite.store.EXPECT().CreateLLMCall(gomock.Any(), gomock.Any()).DoAndReturn(
	// 	func(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	// 		suite.Equal("session-123", call.SessionID)
	// 		return call, nil
	// 	}).Times(2)

	weatherSpec, err := os.ReadFile("./testdata/weather.yaml")
	suite.NoError(err)

	getWeatherAPI := &types.Tool{
		Name:        "getWeather",
		Description: "Weather API service that can be used to retrieve weather information for the given location",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:    ts.URL,
				Schema: string(weatherSpec),
				Query: map[string]string{
					"appid": "secret-key",
				},
				Actions: []*types.ToolAPIAction{
					{
						Name:        "CurrentWeatherData",
						Description: "Call current weather data for one location",
						Method:      "GET",
						Path:        "/weather",
					},
				},
			},
		},
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "what is the capital of united kingdom?",
		},
		{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "The capital of the United Kingdom is London.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What's the weather like there?",
		},
	}

	resp, err := suite.strategy.RunAction(suite.ctx, "session-123", "i-123", getWeatherAPI, history, "CurrentWeatherData")
	suite.NoError(err)

	suite.strategy.wg.Wait()

	spew.Dump(resp)

	suite.True(called, "expected to call the API")

	fmt.Println("U:", history[2].Content)
	fmt.Println("A:", resp.Message)
}
