package model

import (
	"context"
	"fmt"

	"github.com/lukemarsden/helix/api/pkg/types"
)

func GetModel(modelName types.ModelName) (Model, error) {
	if modelName == types.Model_Mistral7b {
		return &Mistral7bInstruct01{}, nil
	} else if modelName == types.Model_SDXL {
		return &SDXL{}, nil
	} else {
		return nil, fmt.Errorf("no model for model name %s", modelName)
	}
}

// rather then keep processing model names from sessions into instances of the model struct
// (just so we can ask it GetMemoryRequirements())
// this gives us an in memory cache of model instances we can quickly lookup from
func GetModels(modelName types.ModelName) (map[types.ModelName]Model, error) {
	models := map[types.ModelName]Model{}
	models[types.Model_Mistral7b] = &Mistral7bInstruct01{}
	models[types.Model_SDXL] = &SDXL{}
	return models, nil
}

func GetModelNameForSession(ctx context.Context, session *types.Session) (types.ModelName, error) {
	if session.Type == "Image" {
		return types.Model_SDXL, nil
	} else if session.Type == "Text" {
		return types.Model_Mistral7b, nil
	}
	return types.Model_None, fmt.Errorf("no model for session type %s", session.Type)
}
