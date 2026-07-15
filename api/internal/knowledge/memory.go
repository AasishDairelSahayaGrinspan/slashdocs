package knowledge

import (
	"context"
	"sort"
	"sync"
)

type Memory struct {
	mu            sync.Mutex
	settings      map[string]Settings
	sources       map[string]Source
	runs          map[string]SyncRun
	deliveries    map[string]bool
	audit         map[string]AuditEvent
	notifications map[string]Notification
	teams         map[string]Team
}

func NewMemory() *Memory {
	return &Memory{
		settings: map[string]Settings{}, sources: map[string]Source{}, runs: map[string]SyncRun{},
		deliveries: map[string]bool{}, audit: map[string]AuditEvent{}, notifications: map[string]Notification{}, teams: map[string]Team{},
	}
}

func (m *Memory) GetSettings(_ context.Context, repositoryID string) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if value, ok := m.settings[repositoryID]; ok {
		return value, nil
	}
	return DefaultSettings(repositoryID), nil
}

func (m *Memory) PutSettings(_ context.Context, value Settings) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value.Portal.Slug = NormalizePortalSlug(value.Portal.Slug)
	if value.Portal.Enabled {
		if value.Portal.Slug == "" {
			return Settings{}, ErrInvalid
		}
		for repositoryID, existing := range m.settings {
			if repositoryID != value.RepositoryID && existing.Portal.Enabled && NormalizePortalSlug(existing.Portal.Slug) == value.Portal.Slug {
				return Settings{}, ErrConflict
			}
		}
	}
	value.UpdatedAt = Now()
	m.settings[value.RepositoryID] = value
	return value, nil
}
func (m *Memory) FindPortal(_ context.Context, slug string) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	slug = NormalizePortalSlug(slug)
	if slug == "" {
		return Settings{}, ErrNotFound
	}
	for _, v := range m.settings {
		if v.Portal.Enabled && NormalizePortalSlug(v.Portal.Slug) == slug {
			return v, nil
		}
	}
	return Settings{}, ErrNotFound
}

func (m *Memory) ListSources(_ context.Context, repositoryID string) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Source{}
	for _, value := range m.sources {
		if value.RepositoryID == repositoryID {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (m *Memory) ListAllSources(_ context.Context) ([]Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0, len(m.sources))
	for _, v := range m.sources {
		out = append(out, v)
	}
	return out, nil
}

func (m *Memory) GetSource(_ context.Context, repositoryID, id string) (Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.sources[id]
	if !ok || v.RepositoryID != repositoryID {
		return Source{}, ErrNotFound
	}
	return v, nil
}

func (m *Memory) PutSource(_ context.Context, value Source) (Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources[value.ID] = value
	return value, nil
}

func (m *Memory) DeleteSource(_ context.Context, repositoryID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.sources[id]
	if !ok || v.RepositoryID != repositoryID {
		return ErrNotFound
	}
	delete(m.sources, id)
	return nil
}

func (m *Memory) AcceptDelivery(_ context.Context, provider, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := provider + ":" + id
	if m.deliveries[k] {
		return false, nil
	}
	m.deliveries[k] = true
	return true, nil
}
func (m *Memory) QueueDelivery(_ context.Context, provider, id string, runs []SyncRun) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := provider + ":" + id
	if m.deliveries[k] {
		return false, nil
	}
	for _, run := range runs {
		if _, exists := m.runs[run.ID]; exists {
			return false, ErrConflict
		}
	}
	m.deliveries[k] = true
	for _, run := range runs {
		m.runs[run.ID] = run
	}
	return true, nil
}
func (m *Memory) AddSyncRun(_ context.Context, v SyncRun) (SyncRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[v.ID] = v
	return v, nil
}
func (m *Memory) ListSyncRuns(_ context.Context, repositoryID string) ([]SyncRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SyncRun{}
	for _, v := range m.runs {
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out, nil
}
func (m *Memory) ClaimQueuedSyncRuns(_ context.Context, limit int) ([]SyncRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SyncRun{}
	for id, v := range m.runs {
		if v.Status == "queued" && len(out) < limit {
			v.Status = "processing"
			m.runs[id] = v
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *Memory) RecoverSyncRuns(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, v := range m.runs {
		if v.Status == "processing" {
			v.Status = "queued"
			m.runs[id] = v
		}
	}
	return nil
}
func (m *Memory) AddAudit(_ context.Context, v AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit[v.ID] = v
	return nil
}
func (m *Memory) ListAudit(_ context.Context, repositoryID string) ([]AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []AuditEvent{}
	for _, v := range m.audit {
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}
func (m *Memory) AddNotification(_ context.Context, v Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications[v.ID] = v
	return nil
}
func (m *Memory) ListNotifications(_ context.Context, userID string) ([]Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Notification{}
	for _, v := range m.notifications {
		if v.UserID == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}
func (m *Memory) SetNotificationRead(_ context.Context, userID, id string, read bool) (Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.notifications[id]
	if !ok || v.UserID != userID {
		return Notification{}, ErrNotFound
	}
	v.Read = read
	m.notifications[id] = v
	return v, nil
}
func (m *Memory) ListTeams(_ context.Context, repositoryID string) ([]Team, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Team{}
	for _, v := range m.teams {
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (m *Memory) GetTeam(_ context.Context, repositoryID, id string) (Team, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.teams[id]
	if !ok || v.RepositoryID != repositoryID {
		return Team{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) PutTeam(_ context.Context, v Team) (Team, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teams[v.ID] = v
	return v, nil
}
func (m *Memory) DeleteTeam(_ context.Context, repositoryID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.teams[id]
	if !ok || v.RepositoryID != repositoryID {
		return ErrNotFound
	}
	delete(m.teams, id)
	return nil
}
