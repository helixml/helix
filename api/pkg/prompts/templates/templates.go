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

//go:embed agent_implementation_approved_push.tmpl
var ImplementationApprovedPushPrompt string

//go:embed agent_rebase_required.tmpl
var RebaseRequiredPrompt string

//go:embed optimus_base_prompt.tmpl
var OptimusTemplate string
