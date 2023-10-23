package controller

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/lukemarsden/helix/api/pkg/types"
)

////////////////////////////////////////////////////////////////////////////////
// LANGUAGE

type LanguageModel struct {
	// INPUTS
	Interactions types.Interactions `json:"interactions"`  // expects user to have given last instruction
	FinetunePath string             `json:"finetune_path"` // path to finetuned model (optional)
	FinetuneFile string             `json:"finetune_file"` // file within above path pointing to specific fine tune file (if applying finetune)
	// OUTPUTS
	DebugStream  chan string
	OutputStream chan string // NB PYTHONUNBUFFERED=1
	FinishChan   chan error
	Status       string `json:"status"` // running, finished, error
}

// TODO: on startup, or when a new instruct session is loaded,

func (l *LanguageModel) Mistral_7B_Instruct_v0_1(ctx context.Context) {

	// l.streamOutput("CATS",
	// 	"bash", "-c", `cat; for i in {1..5}; do echo "debug world $i " >&2; sleep 1; done`)
	// l.streamOutput("CATS", "bash", "-c", `cat`)

	// TODO: convert interactions into the delimited format the instruction
	// tuned model expects with the right tokens and all that

	lastUserMessage := l.Interactions.Messages[len(l.Interactions.Messages)-1].Message
	l.OutputStream <- "🤔... \n\n"
	// l.streamOutput(
	// 	"[INST]"+lastUserMessage+"[/INST]",
	// 	"[/INST]", "</s>",
	// 	"ssh", "-o", "StrictHostKeyChecking=no", "luke@172.17.0.1", `bash -c "
	// 		cd /home/luke/pb/axolotl;
	// 		. venv/bin/activate;
	// 		python -u -m axolotl.cli.inference examples/mistral/qlora-instruct.yml"`,
	// )
	// luke@mind:~/pd/helix$ echo "[INST]i really like you[/INST]" |docker run --gpus all -i quay.io/lukemarsden/axolotl:v0.0.1 python -u -m axolotl.cli.inference examples/mistral/qlora-instruct.yml

	l.streamOutput(
		"[INST]"+lastUserMessage+"[/INST]",
		"[/INST]", "</s>",
		"ssh", "-o", "StrictHostKeyChecking=no", "kai@192.168.86.40", `bash -c "
			docker run --gpus all -i quay.io/lukemarsden/axolotl:v0.0.1 python -u -m axolotl.cli.inference examples/mistral/qlora-instruct.yml"`,
	)
	// echo "prove pythagoras theorem" |  -m axolotl.cli.inference examples/mistral/qlora.yml

}

////////////////////////////////////////////////////////////////////////////////
// TEXT TO IMAGE

type TextToImage struct {
	// INPUTS
	Prompt       string `json:"prompt"` // TODO: add support for negative prompts, other adjustments
	OutputPath   string `json:"output_path"`
	FinetunePath string `json:"finetune_path"` // path to finetuned model (optional)
	FinetuneFile string `json:"finetune_file"` // file within above path pointing to specific fine tune file (if applying finetune)
	// OUTPUTS
	DebugStream  chan string
	OutputStream chan string
	FinishChan   chan error
	Status       string   `json:"status"`        // running, finished, error
	ResultImages []string `json:"result_images"` // filenames relative to OutputPath, only expect this to be filled in when Status == finished
}

// base as opposed to refiner
func (t *TextToImage) SDXL_1_0_Base(ctx context.Context) {
	t.Status = "running"
	for i := 0; i < 5; i++ {
		t.OutputStream <- "hello world "
		time.Sleep(time.Second)
	}
	t.ResultImages = []string{"imagine.jpg"}
	t.Status = "finished"
	t.FinishChan <- nil
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
	FinishChan   chan error
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

func (f *FinetuneLanguageModel) Mistral_7B_Instruct_v0_1(ctx context.Context) {
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
	FinishChan   chan error
	Status       string `json:"status"`      // running, finished, error
	OutputFile   string `json:"output_file"` // a specific e.g. LoRA filename within that directory
}

func (f *FinetuneTextToImage) SDXL_1_0_Base_Finetune(ctx context.Context) {
}

// //////////////////////////////////////////////////////////////////////////////
// PLUMBING

// For testing:
// l.streamOutput("bash", "-c", `for i in {1..5}; do echo "hello world $i "; sleep 1; done`)
// l.streamOutput("bash", "-c", `for i in {1..5}; do echo "debug world $i " >&2; sleep 1; done`)

func (l *LanguageModel) streamOutput(input string, start string, ignore string, command string, args ...string) {
	l.Status = "running"
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		l.DebugStream <- fmt.Sprintf("Error getting stdin pipe: %v", err)
		l.Status = "error"
		l.FinishChan <- err
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		l.DebugStream <- fmt.Sprintf("Error getting stdout pipe: %v", err)
		l.Status = "error"
		l.FinishChan <- err
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		l.DebugStream <- fmt.Sprintf("Error getting stderr pipe: %v", err)
		l.Status = "error"
		l.FinishChan <- err
		return
	}
	if err := cmd.Start(); err != nil {
		l.DebugStream <- fmt.Sprintf("Error starting command: %v", err)
		l.Status = "error"
		l.FinishChan <- err
		return
	}
	go func() {
		foundStartString := false
		scanner := bufio.NewScanner(stdout)
		// scanner.Split(bufio.ScanBytes)
		splitOnSpace := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			if atEOF && len(data) == 0 {
				return 0, nil, nil
			}
			if i := bytes.IndexByte(data, ' '); i >= 0 {
				return i + 1, data[0:i], nil
			}
			if atEOF {
				return len(data), data, nil
			}
			return 0, nil, nil
		}
		scanner.Split(splitOnSpace)
		for scanner.Scan() {
			word := scanner.Text()
			if start == "" || foundStartString {
				word = strings.TrimSuffix(word, ignore)
				l.OutputStream <- word + " "
			} else {
				log.Printf("output: %s", word)
			}
			if strings.HasSuffix(word, start) {
				foundStartString = true
			}
		}
	}()
	go func() {
		errScanner := bufio.NewScanner(stderr)
		for errScanner.Scan() {
			log.Printf("output: %s", errScanner.Text())
			// l.DebugStream <- errScanner.Text()
		}
	}()
	if _, err := stdin.Write([]byte(input)); err != nil {
		l.DebugStream <- fmt.Sprintf("Error writing to stdin: %v", err)
		l.Status = "error"
		l.FinishChan <- err
		return
	}
	if err := stdin.Close(); err != nil {
		l.DebugStream <- fmt.Sprintf("Error closing stdin: %v", err)
		l.Status = "error"
		l.FinishChan <- err
		return
	}
	if err := cmd.Wait(); err != nil {
		l.DebugStream <- fmt.Sprintf("Command finished with error: %v", err)
		l.Status = "error"
		l.FinishChan <- err
		return
	}
	l.Status = "finished"
	l.FinishChan <- nil
}
