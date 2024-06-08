package types

import "testing"

func TestProcessModelName(t *testing.T) {
	type args struct {
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
			name: "empty model, rag",
			args: args{
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
				modelName:   "",
				sessionMode: SessionModeFinetune,
				sessionType: SessionTypeText,
				hasFinetune: true,
				ragEnabled:  false,
			},
			want: Model_Axolotl_Mistral7b,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcessModelName(tt.args.modelName, tt.args.sessionMode, tt.args.sessionType, tt.args.hasFinetune, tt.args.ragEnabled)
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
