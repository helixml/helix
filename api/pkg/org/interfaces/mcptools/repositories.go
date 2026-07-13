package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// Repository tools let a manager Bot (typically Chief of Staff) discover
// the org's Helix git repositories and attach/detach them on other Bots'
// projects so those Bots can clone the code and do real work.
//
// Granted on OwnerBotTools (CoS seed) by default; any Bot can receive
// them via create_bot tools=… or attach_tool.

// --- list_repositories ----------------------------------------------------

const ListRepositoriesName tool.Name = "list_repositories"

type ListRepositories struct{ deps Deps }

func NewListRepositories(deps Deps) *ListRepositories { return &ListRepositories{deps: deps} }

type listRepositoriesArgs struct{}

var listRepositoriesSchema = mustSchema[listRepositoriesArgs]()

func (t *ListRepositories) Name() tool.Name                 { return ListRepositoriesName }
func (t *ListRepositories) InputSchema() *jsonschema.Schema { return listRepositoriesSchema }
func (t *ListRepositories) Description() string {
	return "List every Helix git repository in your organization — id, name, clone URL, " +
		"and whether it is external (GitHub/etc.). Use this before attach_repository so " +
		"you can pick the right repo_id to give a Bot access to the code it needs."
}
func (t *ListRepositories) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID, err := repositoriesOrgID(inv)
	if err != nil {
		return nil, err
	}
	port, err := repositoriesPort(t.deps)
	if err != nil {
		return nil, err
	}
	views, err := port.List(ctx, orgID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return json.Marshal(views)
}

// --- list_bot_repositories ------------------------------------------------

const ListBotRepositoriesName tool.Name = "list_bot_repositories"

type ListBotRepositories struct{ deps Deps }

func NewListBotRepositories(deps Deps) *ListBotRepositories {
	return &ListBotRepositories{deps: deps}
}

type listBotRepositoriesArgs struct {
	BotID string `json:"bot_id"`
}

var listBotRepositoriesSchema = mustSchema[listBotRepositoriesArgs]()

func (t *ListBotRepositories) Name() tool.Name                 { return ListBotRepositoriesName }
func (t *ListBotRepositories) InputSchema() *jsonschema.Schema { return listBotRepositoriesSchema }
func (t *ListBotRepositories) Description() string {
	return "List the git repositories currently attached to a Bot's Helix project " +
		"(the ones its sandbox will clone). Marks primary=true on the project's default " +
		"repo. Prefer this (or get_bot, which also returns repositories) when asked " +
		"\"what repos does this bot have?\". The Bot must have been activated at least " +
		"once so its project exists."
}
func (t *ListBotRepositories) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args listBotRepositoriesArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, errors.New("bot_id is required")
	}
	orgID, err := repositoriesOrgID(inv)
	if err != nil {
		return nil, err
	}
	port, err := repositoriesPort(t.deps)
	if err != nil {
		return nil, err
	}
	views, err := port.ListForBot(ctx, orgID, orgchart.BotID(args.BotID))
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return json.Marshal(views)
}

// --- attach_repository ----------------------------------------------------

const AttachRepositoryName tool.Name = "attach_repository"

type AttachRepository struct{ deps Deps }

func NewAttachRepository(deps Deps) *AttachRepository { return &AttachRepository{deps: deps} }

type attachRepositoryArgs struct {
	BotID   string `json:"bot_id"`
	RepoID  string `json:"repo_id"`
	Primary bool   `json:"primary,omitempty"`
}

var attachRepositorySchema = mustSchema[attachRepositoryArgs]()

func (t *AttachRepository) Name() tool.Name                 { return AttachRepositoryName }
func (t *AttachRepository) InputSchema() *jsonschema.Schema { return attachRepositorySchema }
func (t *AttachRepository) Description() string {
	return "Attach an org git repository to a Bot's Helix project so the Bot's sandbox " +
		"clones it and can work on the code. Pass bot_id (the Bot to equip), repo_id " +
		"(from list_repositories), and optionally primary=true to make it the project's " +
		"default repo. Returns the Bot's updated repository list. The Bot must have been " +
		"activated at least once (so its project exists)."
}
func (t *AttachRepository) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args attachRepositoryArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, errors.New("bot_id is required")
	}
	if args.RepoID == "" {
		return nil, errors.New("repo_id is required")
	}
	orgID, err := repositoriesOrgID(inv)
	if err != nil {
		return nil, err
	}
	port, err := repositoriesPort(t.deps)
	if err != nil {
		return nil, err
	}
	views, err := port.AttachToBot(ctx, orgID, orgchart.BotID(args.BotID), args.RepoID, args.Primary)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return json.Marshal(views)
}

// --- detach_repository ----------------------------------------------------

const DetachRepositoryName tool.Name = "detach_repository"

type DetachRepository struct{ deps Deps }

func NewDetachRepository(deps Deps) *DetachRepository { return &DetachRepository{deps: deps} }

type detachRepositoryArgs struct {
	BotID  string `json:"bot_id"`
	RepoID string `json:"repo_id"`
}

var detachRepositorySchema = mustSchema[detachRepositoryArgs]()

func (t *DetachRepository) Name() tool.Name                 { return DetachRepositoryName }
func (t *DetachRepository) InputSchema() *jsonschema.Schema { return detachRepositorySchema }
func (t *DetachRepository) Description() string {
	return "Detach a git repository from a Bot's Helix project so the Bot's sandbox no " +
		"longer clones it. Pass bot_id and repo_id. Returns the Bot's remaining " +
		"repository list. If the detached repo was primary, the project's default is cleared."
}
func (t *DetachRepository) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args detachRepositoryArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, errors.New("bot_id is required")
	}
	if args.RepoID == "" {
		return nil, errors.New("repo_id is required")
	}
	orgID, err := repositoriesOrgID(inv)
	if err != nil {
		return nil, err
	}
	port, err := repositoriesPort(t.deps)
	if err != nil {
		return nil, err
	}
	views, err := port.DetachFromBot(ctx, orgID, orgchart.BotID(args.BotID), args.RepoID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return json.Marshal(views)
}

// --- helpers --------------------------------------------------------------

func repositoriesOrgID(inv tool.Invocation) (string, error) {
	if inv.Caller == nil {
		return "", errors.New("caller missing on invocation")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return "", errors.New("caller has no organization id")
	}
	return orgID, nil
}

func repositoriesPort(deps Deps) (runtime.Repositories, error) {
	if deps.Repositories == nil {
		return nil, runtime.ErrRepositoriesUnsupported
	}
	return deps.Repositories, nil
}

func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, runtime.ErrBotProjectNotReady) {
		return fmt.Errorf("%w — create/activate the bot first so its Helix project is provisioned", err)
	}
	if errors.Is(err, runtime.ErrRepositoriesUnsupported) {
		return err
	}
	return err
}
