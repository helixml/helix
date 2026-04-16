package server

import "github.com/helixml/helix/api/pkg/types"

// koditEmbeddingDecision records whether each of kodit's embedding providers
// should be configured as an external (admin-chosen) provider proxied through
// Helix, or left on kodit's built-in local default (ONNX for text, SigLIP2
// for vision). External is strictly opt-in: the built-in local models are
// always used when no admin configuration exists.
type koditEmbeddingDecision struct {
	UseExternalText   bool
	UseExternalVision bool
}

// decideKoditEmbedding reads System Settings and returns which embedding
// providers should use the external Helix-proxied path. An external provider
// is only selected when BOTH a provider and a model are configured for that
// embedding type — a half-configured entry is treated as "not configured" so
// the server still boots against built-in models rather than fail against an
// undefined target.
func decideKoditEmbedding(s *types.SystemSettings) koditEmbeddingDecision {
	if s == nil {
		return koditEmbeddingDecision{}
	}
	return koditEmbeddingDecision{
		UseExternalText:   s.KoditTextEmbeddingProvider != "" && s.KoditTextEmbeddingModel != "",
		UseExternalVision: s.KoditVisionEmbeddingProvider != "" && s.KoditVisionEmbeddingModel != "",
	}
}
