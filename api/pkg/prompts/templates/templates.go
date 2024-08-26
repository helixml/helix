package templates

import (
	_ "embed"
)

//go:embed knowledge.tmpl
var KnowledgeTemplate string

//go:embed rag.tmpl
var RagTemplate string

//go:embed finetuning.tmpl
var FinetuningTemplate string
