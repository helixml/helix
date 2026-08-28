package agentdelivery

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/pubsub"
)

const (
	streamName    = "HELIX_ORG_AGENT_ACTIVATIONS"
	subjectPrefix = "helix-org.agent-activations"
	maxRetryDelay = 30 * time.Minute
)

type envelope struct {
	OrganizationID string             `json:"organization_id"`
	AgentID        orgchart.NodeID    `json:"agent_id"`
	Trigger        activation.Trigger `json:"trigger"`
}

// Queue persists one message per agent activation. Each agent has a durable
// pull consumer with one outstanding message, preserving FIFO independently
// of every other agent.
type Queue struct {
	ctx     context.Context
	cancel  context.CancelFunc
	pubsub  pubsub.DurablePubSub
	spawn   activation.Spawn
	logger  *slog.Logger
	mu      sync.Mutex
	active  map[string]struct{}
	removed map[string]struct{}
	publish map[string]*sync.Mutex
}

func New(ctx context.Context, provider pubsub.DurablePubSub, spawn activation.Spawn, logger *slog.Logger) (*Queue, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := provider.EnsurePersistentStream(ctx, streamName, []string{subjectPrefix + ".>"}); err != nil {
		return nil, err
	}
	queueCtx, cancel := context.WithCancel(ctx)
	return &Queue{ctx: queueCtx, cancel: cancel, pubsub: provider, spawn: spawn, logger: logger, active: map[string]struct{}{}, removed: map[string]struct{}{}, publish: map[string]*sync.Mutex{}}, nil
}

func (q *Queue) Close() { q.cancel() }

// Start resumes every durable agent consumer left by an earlier API process.
func (q *Queue) Start() error {
	consumers, err := q.pubsub.ListDurableConsumers(q.ctx, streamName)
	if err != nil {
		return err
	}
	for _, consumer := range consumers {
		if err := q.consume(consumer.Name, consumer.Subject); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) Enqueue(orgID string, agentID orgchart.NodeID, trigger activation.Trigger) {
	if q.spawn == nil {
		return
	}
	subject := subjectFor(orgID, agentID)
	name := consumerName(orgID, agentID)
	lock := q.publishLock(name)
	lock.Lock()
	defer lock.Unlock()
	q.mu.Lock()
	_, removed := q.removed[name]
	q.mu.Unlock()
	if removed {
		return
	}
	if err := q.consume(name, subject); err != nil {
		q.logger.Error("agent delivery: consume", "agent", agentID, "err", err)
		return
	}
	payload, err := json.Marshal(envelope{OrganizationID: orgID, AgentID: agentID, Trigger: trigger})
	if err != nil {
		q.logger.Error("agent delivery: marshal", "agent", agentID, "err", err)
		return
	}
	if err := q.pubsub.PublishDurable(q.ctx, streamName, subject, payload); err != nil {
		q.logger.Error("agent delivery: publish", "agent", agentID, "err", err)
	}
}

func (q *Queue) CleanupAgent(ctx context.Context, orgID string, agentID orgchart.NodeID) error {
	name := consumerName(orgID, agentID)
	lock := q.publishLock(name)
	lock.Lock()
	defer lock.Unlock()

	q.mu.Lock()
	q.removed[name] = struct{}{}
	q.mu.Unlock()
	if err := q.pubsub.DeleteDurableConsumer(ctx, streamName, name); err != nil {
		q.mu.Lock()
		delete(q.removed, name)
		q.mu.Unlock()
		return err
	}
	q.mu.Lock()
	delete(q.active, name)
	q.mu.Unlock()
	if err := q.pubsub.PurgeDurableSubject(ctx, streamName, subjectFor(orgID, agentID)); err != nil {
		q.mu.Lock()
		delete(q.removed, name)
		q.mu.Unlock()
		return err
	}
	return nil
}

// CancelOutstanding discards queued and in-flight deliveries without marking
// the agent removed, so Restart can enqueue a clean activation immediately.
func (q *Queue) CancelOutstanding(ctx context.Context, orgID string, agentID orgchart.NodeID) error {
	name := consumerName(orgID, agentID)
	lock := q.publishLock(name)
	lock.Lock()
	defer lock.Unlock()

	if err := q.pubsub.DeleteDurableConsumer(ctx, streamName, name); err != nil {
		return err
	}
	q.mu.Lock()
	delete(q.active, name)
	q.mu.Unlock()
	return q.pubsub.PurgeDurableSubject(ctx, streamName, subjectFor(orgID, agentID))
}

func (q *Queue) RestoreAgent(orgID string, agentID orgchart.NodeID) {
	name := consumerName(orgID, agentID)
	lock := q.publishLock(name)
	lock.Lock()
	defer lock.Unlock()
	q.mu.Lock()
	delete(q.removed, name)
	q.mu.Unlock()
}

func (q *Queue) publishLock(name string) *sync.Mutex {
	q.mu.Lock()
	defer q.mu.Unlock()
	if lock, ok := q.publish[name]; ok {
		return lock
	}
	lock := &sync.Mutex{}
	q.publish[name] = lock
	return lock
}

func (q *Queue) consume(name, subject string) error {
	q.mu.Lock()
	if _, ok := q.active[name]; ok {
		q.mu.Unlock()
		return nil
	}
	q.active[name] = struct{}{}
	q.mu.Unlock()

	_, err := q.pubsub.ConsumeDurable(q.ctx, streamName, name, subject, 30*time.Minute, func(msg *pubsub.Message) error {
		q.handle(msg)
		return nil
	})
	if err != nil {
		q.mu.Lock()
		delete(q.active, name)
		q.mu.Unlock()
		return err
	}
	return nil
}

func (q *Queue) handle(msg *pubsub.Message) {
	var delivery envelope
	if err := json.Unmarshal(msg.Data, &delivery); err != nil {
		q.logger.Error("agent delivery: decode", "err", err)
		_ = msg.NakWithDelay(retryDelay(msg.NumDelivered))
		return
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = msg.InProgress()
			case <-done:
				return
			}
		}
	}()
	err := q.spawn(context.Background(), delivery.OrganizationID, delivery.AgentID, []activation.Trigger{delivery.Trigger})
	close(done)
	if err != nil {
		q.logger.Warn("agent delivery: activation failed", "agent", delivery.AgentID, "err", err)
		_ = msg.NakWithDelay(retryDelay(msg.NumDelivered))
		return
	}
	if err := msg.Ack(); err != nil {
		q.logger.Error("agent delivery: ack", "agent", delivery.AgentID, "err", err)
	}
}

func retryDelay(numDelivered uint64) time.Duration {
	delay := time.Second
	for delivered := uint64(1); delivered < numDelivered && delay < maxRetryDelay; delivered++ {
		delay *= 2
	}
	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

func subjectFor(orgID string, agentID orgchart.NodeID) string {
	encode := base64.RawURLEncoding.EncodeToString
	return subjectPrefix + "." + encode([]byte(orgID)) + "." + encode([]byte(agentID))
}

func consumerName(orgID string, agentID orgchart.NodeID) string {
	sum := sha256.Sum256([]byte(orgID + "\x00" + string(agentID)))
	return "helix-org-agent-" + hex.EncodeToString(sum[:16])
}
