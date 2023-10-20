package model

import (
	"context"
	"fmt"

	"github.com/lukemarsden/helix/api/pkg/types"
)

// given a model name - reutrn the correct language model
func GetLanguageModel(model types.ModelName) (LanguageModel, error) {
	if model == types.Model_Mistral7b {
		return &Mistral7bInstruct01{}, nil
	}
	return nil, fmt.Errorf("no model for model name %s", model)
}

func GetImageModel(model types.ModelName) (ImageModel, error) {
	if model == types.Model_SDXL {
		return &SDXL{}, nil
	}
	return nil, fmt.Errorf("no model for model name %s", model)
}

func GetModelNameForSession(ctx context.Context, session *types.Session) (types.ModelName, error) {
	if session.Type == "Image" {
		return types.Model_SDXL, nil
	} else if session.Type == "Text" {
		return types.Model_Mistral7b, nil
	}
	return types.Model_None, fmt.Errorf("no model for session type %s", session.Type)
}
