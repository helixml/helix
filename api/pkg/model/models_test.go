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
		hasFinetune bool
		ragEnabled  bool
	}
	tests := []struct {
		name    string
		args    args
		want    ModelName
		wantErr bool
	}{
		{
			name: "togetherai meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
			args: args{
				provider:    "togetherai",
				modelName:   "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				sessionMode: types.SessionModeFinetune,
				sessionType: types.SessionTypeText,
				hasFinetune: false,
				ragEnabled:  true,
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
				hasFinetune: false,
				ragEnabled:  true,
			},
			want: NewModel(Model_Ollama_Llama31_8b),
		},
		{
			name: "empty model, finetune, no rag",
			args: args{
				provider:    "helix",
				modelName:   "",
				sessionMode: types.SessionModeFinetune,
				sessionType: types.SessionTypeText,
				hasFinetune: true,
				ragEnabled:  false,
			},
			want: NewModel(Model_Axolotl_Mistral7b),
		},
		{
			name: "normal inference",
			args: args{
				provider:    "helix",
				modelName:   "",
				sessionMode: types.SessionModeInference,
				sessionType: types.SessionTypeText,
				hasFinetune: false,
				ragEnabled:  false,
			},
			want: NewModel(Model_Ollama_Llama31_8b),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcessModelName(tt.args.provider, tt.args.modelName, tt.args.sessionMode, tt.args.sessionType, tt.args.hasFinetune, tt.args.ragEnabled)
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
