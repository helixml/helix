package evals

import (
	"log"

	"github.com/helixml/helix/api/pkg/types"
)

type EvalSummary struct {
	EvalId   string
	Score    float32 // avg of scores
	Sessions []types.Session
}

type Evals struct {
	// TODO: loaded env vars, store instance etc
}

func NewEvals() (*Evals, error) {
	// implementation for NewEvals
	return &Evals{}, nil
}

func (e *Evals) GetEvalSessions(evalId string) ([]types.Session, error) {
	// implementation for GetEvalSessions
	return []types.Session{}, nil
}

func (e *Evals) GetBaseSessions() ([]types.Session, error) {
	// implementation for GetBaseSessions
	return []types.Session{}, nil
}

func (e *Evals) Init() (string, error) {
	// implementation for Init
	return "", nil
}

func (e *Evals) ListEvalSummary() ([]EvalSummary, error) {
	// implementation for ListEvalSummary
	return []EvalSummary{}, nil
}

func (e *Evals) Run(evalId string) error {
	log.Printf("hello from evals")
	// implementation for Run
	return nil
}
