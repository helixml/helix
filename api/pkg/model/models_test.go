package model

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

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
