package controller

import (
	"context"
	"time"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

////////////////////////////////////////////////////////////////////////////////
// TEXT TO IMAGE

type TextToImage struct {
	// INPUTS
	Prompt     string `json:"prompt"` // TODO: add support for negative prompts, other adjustments
	OutputPath string `json:"output_path"`
	// OUTPUTS
	DebugStream  chan string
	OutputStream chan string
	Status       string   `json:"status"`        // running, finished, error
	ResultImages []string `json:"result_images"` // filenames relative to OutputPath, only expect this to be filled in when Status == finished
}

// base as opposed to refiner
func (t *TextToImage) SDXL_1_0_Base(ctx context.Context) error {
	t.Status = "running"
	for i := 0; i < 5; i++ {
		t.OutputStream <- "hello world"
		time.Sleep(time.Second)
	}
	t.ResultImages = []string{"imagine.jpg"}
	t.Status = "finished"
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// LANGUAGE

type LanguageModel struct {
	// INPUTS
	Interactions types.Interactions `json:"interactions"` // expects user to have given last instruction
	// OUTPUTS
	DebugStream  chan string
	OutputStream chan string // NB PYTHONUNBUFFERED=1
	Status       string      `json:"status"` // running, finished, error
}

func (l *LanguageModel) Mistral_7B_Instruct_v0_1(ctx context.Context) error {
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// FINE TUNE LANGUAGE

type FinetuneLanguageModel struct {
	// INPUTS
	InputDataset ShareGPT `json:"input_dataset"` // literal input training dataset - https://github.com/OpenAccess-AI-Collective/axolotl#dataset
	OutputPath   string   `json:"output_path"`   // path to resulting directory
	// OUTPUTS
	DebugStream  chan string
	OutputStream chan string
	Status       string `json:"status"`      // running, finished, error
	OutputFile   string `json:"output_file"` // a specific e.g. LoRA filename within the given output directory
}

// input data format (maybe move this on disk if they get big enough)
type ShareGPT struct {
	Conversations []struct {
		From  string `json:"from"`
		Value string `json:"value"`
	} `json:"conversations"`
}

func (f *FinetuneTextToImage) Mistral_7B_Instruct_v0_1(ctx context.Context) error {
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// FINE TUNE TEXT TO IMAGE

type FinetuneTextToImage struct {
	// INPUTS
	InputPath  string `json:"input_path"`  // path to directory containing file_1.png and file_1.txt captions
	OutputPath string `json:"output_path"` // path to resulting directory
	// OUTPUTS
	DebugStream  chan string
	OutputStream chan string
	Status       string `json:"status"`      // running, finished, error
	OutputFile   string `json:"output_file"` // a specific e.g. LoRA filename within that directory
}

func (f *FinetuneTextToImage) SDXL_1_0_Base_Finetune(ctx context.Context) error {
	return nil
}
