// Package koditutil provides embedding support for kodit's in-process library.
// It wraps the hugot Go ONNX runtime to load the st-codesearch-distilroberta-base
// model from a directory on disk, avoiding the need for kodit's embed_model build tag.
package koditutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"

	"github.com/helixml/kodit/infrastructure/provider"
)

const batchMax = 32

// singleton holds the process-wide hugot session and pipeline.
// hugot/ONNX only allows one active session per process.
var singleton struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
	mu       sync.Mutex
	ready    bool
}

// DiskEmbedder loads the ONNX embedding model from a directory on disk and
// implements provider.Embedder so it can be passed to kodit.WithEmbeddingProvider().
type DiskEmbedder struct {
	modelDir string
}

// NewDiskEmbedder creates an embedder that loads model files from modelDir.
// modelDir should contain the model subdirectory (e.g. flax-sentence-embeddings_st-codesearch-distilroberta-base/)
// with tokenizer.json and onnx/model.onnx inside it.
func NewDiskEmbedder(modelDir string) *DiskEmbedder {
	return &DiskEmbedder{modelDir: modelDir}
}

// Available reports whether the model directory exists and looks valid.
func (d *DiskEmbedder) Available() bool {
	modelPath, err := d.resolveModelPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(modelPath, "tokenizer.json"))
	return err == nil
}

func (d *DiskEmbedder) resolveModelPath() (string, error) {
	entries, err := os.ReadDir(d.modelDir)
	if err != nil {
		return "", fmt.Errorf("read model dir %s: %w", d.modelDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(d.modelDir, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("no model subdirectory found in %s", d.modelDir)
}

func (d *DiskEmbedder) initialize() error {
	singleton.mu.Lock()
	defer singleton.mu.Unlock()

	if singleton.ready {
		return nil
	}

	session, err := hugot.NewGoSession()
	if err != nil {
		return fmt.Errorf("create hugot session: %w", err)
	}

	modelPath, err := d.resolveModelPath()
	if err != nil {
		_ = session.Destroy()
		return err
	}

	config := hugot.FeatureExtractionConfig{
		ModelPath: modelPath,
		Name:      "helix-embeddings",
		Options: []hugot.FeatureExtractionOption{
			pipelines.WithNormalization(),
		},
	}
	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		_ = session.Destroy()
		return fmt.Errorf("create feature extraction pipeline: %w", err)
	}

	singleton.session = session
	singleton.pipeline = pipeline
	singleton.ready = true
	return nil
}

// Embed generates embeddings for the given texts using the local ONNX model.
func (d *DiskEmbedder) Embed(ctx context.Context, req provider.EmbeddingRequest) (provider.EmbeddingResponse, error) {
	texts := req.Texts()
	if len(texts) == 0 {
		return provider.NewEmbeddingResponse([][]float64{}, provider.NewUsage(0, 0, 0)), nil
	}

	if err := ctx.Err(); err != nil {
		return provider.EmbeddingResponse{}, err
	}

	if err := d.initialize(); err != nil {
		return provider.EmbeddingResponse{}, fmt.Errorf("initialize hugot: %w", err)
	}

	singleton.mu.Lock()
	defer singleton.mu.Unlock()

	embeddings := make([][]float64, 0, len(texts))

	for i := 0; i < len(texts); i += batchMax {
		if err := ctx.Err(); err != nil {
			return provider.EmbeddingResponse{}, err
		}

		end := min(i+batchMax, len(texts))
		batch := texts[i:end]

		result, err := singleton.pipeline.RunPipeline(batch)
		if err != nil {
			return provider.EmbeddingResponse{}, fmt.Errorf("run embedding pipeline: %w", err)
		}

		for _, vec32 := range result.Embeddings {
			vec64 := make([]float64, len(vec32))
			for j, v := range vec32 {
				vec64[j] = float64(v)
			}
			embeddings = append(embeddings, vec64)
		}
	}

	return provider.NewEmbeddingResponse(embeddings, provider.NewUsage(0, 0, 0)), nil
}

var _ provider.Embedder = (*DiskEmbedder)(nil)
