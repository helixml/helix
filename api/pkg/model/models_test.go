package model

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestParseProviderFromModel(t *testing.T) {
	tests := []struct {
		input        string
		wantProvider string
		wantModel    string
	}{
		{"openrouter/gpt-4", "openrouter", "gpt-4"},
		{"openrouter/x-ai/glm-4.6", "openrouter", "x-ai/glm-4.6"},
		{"helix/llama3:instruct", "helix", "llama3:instruct"},
		{"gpt-4", "", "gpt-4"},
		{"llama3:instruct", "", "llama3:instruct"},
		{"meta-llama/Meta-Llama-3.1-8B", "meta-llama", "Meta-Llama-3.1-8B"},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			provider, model := ParseProviderFromModel(tt.input)
			if provider != tt.wantProvider {
				t.Errorf("ParseProviderFromModel() provider = %q, want %q", provider, tt.wantProvider)
			}
			if model != tt.wantModel {
				t.Errorf("ParseProviderFromModel() model = %q, want %q", model, tt.wantModel)
			}
		})
	}
}

func TestProcessModelName(t *testing.T) {
	type args struct {
		provider    string
		modelName   string
		sessionMode types.SessionMode
		sessionType types.SessionType
	}
	tests := []struct {
		name    string
		args    args
		want    Name
		wantErr bool
	}{
		{
			name: "togetherai meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
			args: args{
				provider:    "togetherai",
				modelName:   "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				sessionMode: types.SessionModeFinetune,
				sessionType: types.SessionTypeText,
			},
			want: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		},
		{
			name: "empty model, rag",
			args: args{
				provider:    "helix",
				modelName:   "",
				sessionMode: types.SessionModeFinetune,
				sessionType: types.SessionTypeText,
			},
			want: NewModel(ModelOllamaLlama38b),
		},
		{
			name: "normal inference",
			args: args{
				provider:    "helix",
				modelName:   "",
				sessionMode: types.SessionModeInference,
				sessionType: types.SessionTypeText,
			},
			want: NewModel(ModelOllamaLlama38b),
		},
		{
			name: "normal inference, model set helix-4",
			args: args{
				provider:    "helix",
				modelName:   "helix-4",
				sessionMode: types.SessionModeInference,
				sessionType: types.SessionTypeText,
			},
			want: NewModel(ModelOllamaLlama370b),
		},
		{
			name: "normal inference, model set helix-mixtral",
			args: args{
				provider:    "helix",
				modelName:   "helix-mixtral",
				sessionMode: types.SessionModeInference,
				sessionType: types.SessionTypeText,
			},
			want: NewModel(ModelOllamaMixtral),
		},
		{
			name: "external agent model",
			args: args{
				provider:    "helix",
				modelName:   "external_agent",
				sessionMode: types.SessionModeInference,
				sessionType: types.SessionTypeText,
			},
			want: NewModel("external_agent"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcessModelName(tt.args.provider, tt.args.modelName, tt.args.sessionType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessModelName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want.String() {
				t.Errorf("ProcessModelName() = %v, want %v", got, tt.want.String())
			}
		})
	}
}
