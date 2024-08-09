package types

import "testing"

func TestProcessModelName(t *testing.T) {
	type args struct {
		provider    string
		modelName   string
		sessionMode SessionMode
		sessionType SessionType
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
				sessionMode: SessionModeFinetune,
				sessionType: SessionTypeText,
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
				sessionMode: SessionModeFinetune,
				sessionType: SessionTypeText,
				hasFinetune: false,
				ragEnabled:  true,
			},
			want: Model_Ollama_Llama3_8b,
		},
		{
			name: "empty model, finetune, no rag",
			args: args{
				provider:    "helix",
				modelName:   "",
				sessionMode: SessionModeFinetune,
				sessionType: SessionTypeText,
				hasFinetune: true,
				ragEnabled:  false,
			},
			want: Model_Axolotl_Mistral7b,
		},
		{
			name: "normal inference",
			args: args{
				provider:    "helix",
				modelName:   "",
				sessionMode: SessionModeInference,
				sessionType: SessionTypeText,
				hasFinetune: false,
				ragEnabled:  false,
			},
			want: Model_Ollama_Llama3_8b,
		},
		{
			name: "normal inference, model set helix-4",
			args: args{
				provider:    "helix",
				modelName:   "helix-4",
				sessionMode: SessionModeInference,
				sessionType: SessionTypeText,
				hasFinetune: false,
				ragEnabled:  false,
			},
			want: Model_Ollama_Llama3_70b,
		},
		{
			name: "normal inference, model set helix-mixtral",
			args: args{
				provider:    "helix",
				modelName:   "helix-mixtral",
				sessionMode: SessionModeInference,
				sessionType: SessionTypeText,
				hasFinetune: false,
				ragEnabled:  false,
			},
			want: Model_Ollama_Mixtral,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcessModelName(tt.args.provider, tt.args.modelName, tt.args.sessionMode, tt.args.sessionType, tt.args.hasFinetune, tt.args.ragEnabled)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessModelName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ProcessModelName() = %v, want %v", got, tt.want)
			}
		})
	}
}
