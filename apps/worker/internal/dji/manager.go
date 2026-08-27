package dji

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type ManagedSession interface {
	Events() <-chan SessionEvent
	Done() <-chan struct{}
}

type SessionConnector func(context.Context, MQTTConfig, MQTTMessageHandler) (ManagedSession, error)
type MessageHandlerFactory func(AdapterLease) MQTTMessageHandler

type activeAdapter struct {
	lease   AdapterLease
	cancel  context.CancelFunc
	allDone chan struct{}
}

type AdapterManager struct {
	repository LeaseRepository
	resolver   SecretResolver
	connect    SessionConnector
	handler    MessageHandlerFactory
	owner      string
	logger     *slog.Logger
	lease      time.Duration
	maxActive  int
	mu         sync.Mutex
	active     map[int64]activeAdapter
}

func NewAdapterManager(
	repository LeaseRepository, resolver SecretResolver, connector SessionConnector,
	handler MessageHandlerFactory, owner string, logger *slog.Logger,
) *AdapterManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &AdapterManager{
		repository: repository, resolver: resolver, connect: connector, handler: handler,
		owner: owner, logger: logger, lease: 30 * time.Second, maxActive: 32,
		active: make(map[int64]activeAdapter),
	}
}

func (manager *AdapterManager) watch(ctx context.Context, lease AdapterLease, session ManagedSession, done chan struct{}) {
	defer close(done)
	for {
		select {
		case event := <-session.Events():
			status := event.State
			if status != "connected" && status != "degraded" && status != "failed" {
				status = "degraded"
			}
			if err := manager.repository.UpdateStatus(ctx, lease, manager.owner, status, event.Code); err != nil && ctx.Err() == nil {
				manager.logger.Error("DJI adapter status update failed", "adapter_id", lease.AdapterID, "error", err.Error())
			}
		case <-session.Done():
			return
		case <-ctx.Done():
			return
		}
	}
}

func (manager *AdapterManager) start(ctx context.Context, lease AdapterLease) error {
	config, err := BuildMQTTConfig(ctx, lease, manager.resolver)
	if err != nil {
		_ = manager.repository.UpdateStatus(ctx, lease, manager.owner, "failed", err.Error())
		_ = manager.repository.Release(ctx, lease, manager.owner)
		return err
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	handler := MQTTMessageHandler(nil)
	if manager.handler != nil {
		handler = manager.handler(lease)
	}
	session, err := manager.connect(sessionCtx, config, handler)
	for index := range config.Password {
		config.Password[index] = 0
	}
	if err != nil {
		cancel()
		_ = manager.repository.UpdateStatus(ctx, lease, manager.owner, "failed", "DJI_MQTT_START_FAILED")
		_ = manager.repository.Release(ctx, lease, manager.owner)
		return err
	}
	done := make(chan struct{})
	manager.mu.Lock()
	manager.active[lease.AdapterID] = activeAdapter{lease: lease, cancel: cancel, allDone: done}
	manager.mu.Unlock()
	go manager.watch(sessionCtx, lease, session, done)
	return nil
}

func (manager *AdapterManager) reconcile(ctx context.Context) error {
	manager.mu.Lock()
	active := make([]activeAdapter, 0, len(manager.active))
	for _, adapter := range manager.active {
		active = append(active, adapter)
	}
	manager.mu.Unlock()
	var problems []error
	for _, adapter := range active {
		renewed, err := manager.repository.Renew(ctx, adapter.lease, manager.owner, manager.lease)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if !renewed {
			adapter.cancel()
			manager.mu.Lock()
			delete(manager.active, adapter.lease.AdapterID)
			manager.mu.Unlock()
		}
	}
	manager.mu.Lock()
	available := manager.maxActive - len(manager.active)
	manager.mu.Unlock()
	if available <= 0 {
		return errors.Join(problems...)
	}
	leases, err := manager.repository.Claim(ctx, manager.owner, available, manager.lease)
	if err != nil {
		return errors.Join(append(problems, err)...)
	}
	for _, lease := range leases {
		if err := manager.start(ctx, lease); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func (manager *AdapterManager) Run(ctx context.Context) error {
	if manager.repository == nil || manager.resolver == nil || manager.connect == nil || manager.owner == "" {
		return errors.New("DJI_ADAPTER_MANAGER_INVALID")
	}
	ticker := time.NewTicker(manager.lease / 3)
	defer ticker.Stop()
	for {
		if err := manager.reconcile(ctx); err != nil && ctx.Err() == nil {
			manager.logger.Error("DJI adapter reconciliation failed", "error", err.Error())
		}
		select {
		case <-ctx.Done():
			manager.mu.Lock()
			active := make([]activeAdapter, 0, len(manager.active))
			for _, adapter := range manager.active {
				adapter.cancel()
				active = append(active, adapter)
			}
			manager.active = make(map[int64]activeAdapter)
			manager.mu.Unlock()
			for _, adapter := range active {
				<-adapter.allDone
				_ = manager.repository.Release(context.Background(), adapter.lease, manager.owner)
			}
			return nil
		case <-ticker.C:
		}
	}
}
