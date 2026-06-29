package yellowdog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// node mirrors the YD scheduler `Node` entity returned by
// GET /workerPools/nodes. We only decode the fields we use.
type node struct {
	ID      string      `json:"id"`
	Status  string      `json:"status"`
	Details nodeDetails `json:"details"`
}

type nodeDetails struct {
	WorkerTag    string `json:"workerTag"`
	InstanceType string `json:"instanceType"`
}

// sliceNode is the paginated envelope YD returns. We only read the
// first page: discovery doesn't need full pagination since healthy
// deployments routinely have <10 online nodes, well under the 200 page
// size we request.
type sliceNode struct {
	Items []node `json:"items"`
}

// NodePool is a distinct group of online nodes sharing a worker tag and
// instance type - i.e. one provisioned YD pool as Helix observes it from
// live nodes. NodeCount is how many RUNNING nodes back it right now.
type NodePool struct {
	WorkerTag    string
	InstanceType string
	NodeCount    int
}

// fetchOnlineNodes queries the YD scheduler API for the currently-online
// (RUNNING) nodes visible to the configured API key. Shared by the tag
// and pool discovery helpers.
func fetchOnlineNodes(ctx context.Context, cfg Config) ([]node, error) {
	creds := credentials{keyID: cfg.APIKeyID, secret: cfg.APISecret}
	if !creds.valid() {
		return nil, errors.New("yellowdog: discovery requires APIKeyID and APISecret")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	if !cfg.AllowInsecureBaseURL && !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("yellowdog: discovery BaseURL must use https:// (got %q)", base)
	}
	httpc := cfg.HTTPClient
	if httpc == nil {
		httpc = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	// nodeSearch and sliceReference are both *required* query params on
	// GET /workerPools/nodes per the scheduler OpenAPI; the server 400s
	// without them. Both serialise as URL-encoded JSON strings, matching
	// the convention searchWorkRequirements already uses for
	// /work/requirements.
	q := url.Values{}
	q.Set("nodeSearch", `{"statuses":["RUNNING"]}`)
	q.Set("sliceReference", `{"size":200}`)

	var out sliceNode
	if err := doJSON(ctx, httpc, creds, base, http.MethodGet, "/workerPools/nodes", q, nil, &out, retryConfig{maxAttempts: 3}); err != nil {
		return nil, fmt.Errorf("yellowdog: query nodes: %w", err)
	}
	return out.Items, nil
}

// DiscoverOnlineWorkerTags queries the YD scheduler API for the set of
// distinct workerTags advertised by currently-online (RUNNING) nodes
// visible to the configured API key.
//
// Bootstrap callers use this to populate HELIX_YD_WORKER_TAG from the
// operator's pre-existing pool, instead of guessing `worker-<namespace>`
// (which silently mismatches whenever the pool was set up with a
// different convention - the YD POC `config.toml`, for example, derives
// its tag from `{{username}}` not `{{namespace}}`).
//
// Returns a sorted list of distinct non-empty tags. Empty list means
// "no online nodes visible to this key" (likely no pool provisioned
// yet); caller decides whether that's an error or whether to fall back
// to a default.
//
// Assumption: this API key's visibility scope is one Helix install's
// pool(s). YD scopes by key, not by namespace path; an operator who
// uses one key across multiple Helix deployments' namespaces will get
// the union of all tags and must set HELIX_YD_WORKER_TAG explicitly to
// disambiguate (see bootstrap's >1-tag handling).
func DiscoverOnlineWorkerTags(ctx context.Context, cfg Config) ([]string, error) {
	nodes, err := fetchOnlineNodes(ctx, cfg)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, n := range nodes {
		tag := strings.TrimSpace(n.Details.WorkerTag)
		if tag == "" {
			continue
		}
		seen[tag] = struct{}{}
	}
	tags := make([]string, 0, len(seen))
	for t := range seen {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags, nil
}

// DiscoverNodePools groups currently-online (RUNNING) nodes by
// (workerTag, instanceType): the set of provisioned pools Helix can see
// from live nodes, with the count of nodes backing each. This is the
// discovery input for maintaining a floor per pool WITHOUT the operator
// declaring pools in config - whatever they brought up with yd-provision
// shows up here, keyed by the tag a work requirement must target and the
// instance type a caller maps to an accelerator.
//
// Nodes missing a workerTag are skipped: a WR cannot target them. Same
// visibility caveat as DiscoverOnlineWorkerTags - the key scopes what is
// seen. Sorted by workerTag then instanceType for stable output.
func DiscoverNodePools(ctx context.Context, cfg Config) ([]NodePool, error) {
	nodes, err := fetchOnlineNodes(ctx, cfg)
	if err != nil {
		return nil, err
	}
	type key struct{ tag, itype string }
	counts := map[key]int{}
	for _, n := range nodes {
		tag := strings.TrimSpace(n.Details.WorkerTag)
		if tag == "" {
			continue
		}
		counts[key{tag, strings.TrimSpace(n.Details.InstanceType)}]++
	}
	pools := make([]NodePool, 0, len(counts))
	for k, c := range counts {
		pools = append(pools, NodePool{WorkerTag: k.tag, InstanceType: k.itype, NodeCount: c})
	}
	sort.Slice(pools, func(i, j int) bool {
		if pools[i].WorkerTag != pools[j].WorkerTag {
			return pools[i].WorkerTag < pools[j].WorkerTag
		}
		return pools[i].InstanceType < pools[j].InstanceType
	})
	return pools, nil
}
