package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/ninadsindu/luma-gpu-control-plane/internal/domain"
)

const postgresDriverName = "pgx"

type PostgresStore struct {
	db *sql.DB
}

var _ Store = (*PostgresStore)(nil)

func NewPostgresStore(ctx context.Context, dsn string, seed bool) (*PostgresStore, error) {
	db, err := sql.Open(postgresDriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	store := &PostgresStore{db: db}
	if err := store.pingWithRetry(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.bootstrap(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if seed {
		empty, err := store.isEmpty(ctx)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		if empty {
			if _, err := store.SeedDemoData(); err != nil {
				_ = db.Close()
				return nil, err
			}
		}
	}
	return store, nil
}

func (s *PostgresStore) pingWithRetry(ctx context.Context) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	backoff := 150 * time.Millisecond
	for {
		if err := s.db.PingContext(deadlineCtx); err == nil {
			return nil
		} else if deadlineCtx.Err() != nil {
			return err
		}

		select {
		case <-time.After(backoff):
		case <-deadlineCtx.Done():
			return deadlineCtx.Err()
		}

		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
}

func (s *PostgresStore) bootstrap(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			gpu_type TEXT NOT NULL,
			total_gpus INTEGER NOT NULL,
			allocated_gpus INTEGER NOT NULL,
			region TEXT NOT NULL,
			data_center TEXT NOT NULL,
			zone TEXT NOT NULL,
			provider TEXT NOT NULL,
			capacity_class TEXT NOT NULL,
			health TEXT NOT NULL,
			running_workload_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS workloads (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			gpu_type TEXT NOT NULL,
			gpu_count INTEGER NOT NULL,
			priority TEXT NOT NULL,
			duration_seconds INTEGER NOT NULL,
			spot_tolerant BOOLEAN NOT NULL,
			resumable BOOLEAN NOT NULL DEFAULT FALSE,
			replicas INTEGER NOT NULL DEFAULT 1,
			replica_placements JSONB NOT NULL DEFAULT '[]'::jsonb,
			state TEXT NOT NULL,
			placement JSONB,
			status_reason TEXT NOT NULL DEFAULT '',
			scheduling_explanation TEXT NOT NULL DEFAULT '',
			preempt_notice_seconds INTEGER NOT NULL DEFAULT 0,
			drain_started_at TIMESTAMPTZ,
			checkpoint_state TEXT NOT NULL DEFAULT '',
			resume_eligible BOOLEAN NOT NULL DEFAULT FALSE,
			submitted_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`ALTER TABLE workloads ADD COLUMN IF NOT EXISTS resumable BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE workloads ADD COLUMN IF NOT EXISTS replicas INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE workloads ADD COLUMN IF NOT EXISTS replica_placements JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE workloads ADD COLUMN IF NOT EXISTS preempt_notice_seconds INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE workloads ADD COLUMN IF NOT EXISTS drain_started_at TIMESTAMPTZ`,
		`ALTER TABLE workloads ADD COLUMN IF NOT EXISTS checkpoint_state TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workloads ADD COLUMN IF NOT EXISTS resume_eligible BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			timestamp TIMESTAMPTZ NOT NULL,
			type TEXT NOT NULL,
			actor TEXT NOT NULL,
			workload_id TEXT NOT NULL DEFAULT '',
			node_id TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL,
			metadata JSONB
		)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) isEmpty(ctx context.Context) (bool, error) {
	var nodes, workloads, events int
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM nodes),
			(SELECT COUNT(*) FROM workloads),
			(SELECT COUNT(*) FROM events)
	`).Scan(&nodes, &workloads, &events); err != nil {
		return false, err
	}
	return nodes+workloads+events == 0, nil
}

func (s *PostgresStore) SeedDemoData() (DemoDataSummary, error) {
	state := NewSeededMemoryStore()
	summary := DemoDataSummary{
		Nodes:     len(state.nodes),
		Workloads: len(state.workloads),
		Events:    len(state.events),
	}
	if err := s.replaceAll(context.Background(), state); err != nil {
		return DemoDataSummary{}, err
	}
	return summary, nil
}

func (s *PostgresStore) Clear() (DemoDataSummary, error) {
	ctx := context.Background()
	summary, err := s.countSummary(ctx)
	if err != nil {
		return DemoDataSummary{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DemoDataSummary{}, err
	}
	defer rollback(tx)

	for _, stmt := range []string{
		`DELETE FROM events`,
		`DELETE FROM workloads`,
		`DELETE FROM nodes`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return DemoDataSummary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DemoDataSummary{}, err
	}
	return summary, nil
}

func (s *PostgresStore) CreateWorkload(workload domain.Workload) (domain.Workload, error) {
	var created domain.Workload
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		created, err = state.CreateWorkload(workload)
		return err
	}, nil)
	return created, err
}

func (s *PostgresStore) GetWorkload(id string) (domain.Workload, bool) {
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, gpu_type, gpu_count, priority, duration_seconds, spot_tolerant, resumable, replicas, replica_placements, state, placement, status_reason, scheduling_explanation, preempt_notice_seconds, drain_started_at, checkpoint_state, resume_eligible, submitted_at, updated_at
		FROM workloads
		WHERE id = $1
	`, id)
	if err != nil {
		return domain.Workload{}, false
	}
	defer rows.Close()
	workloads, err := scanWorkloads(rows)
	if err != nil || len(workloads) == 0 {
		return domain.Workload{}, false
	}
	return workloads[0], true
}

func (s *PostgresStore) ListWorkloads() []domain.Workload {
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, gpu_type, gpu_count, priority, duration_seconds, spot_tolerant, resumable, replicas, replica_placements, state, placement, status_reason, scheduling_explanation, preempt_notice_seconds, drain_started_at, checkpoint_state, resume_eligible, submitted_at, updated_at
		FROM workloads
		ORDER BY submitted_at, id
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	workloads, err := scanWorkloads(rows)
	if err != nil {
		return nil
	}
	return workloads
}

func (s *PostgresStore) UpdateWorkload(id string, fn func(*domain.Workload) error) (domain.Workload, error) {
	var updated domain.Workload
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		updated, err = state.UpdateWorkload(id, fn)
		return err
	}, nil)
	return updated, err
}

func (s *PostgresStore) CompleteExpiredWorkloads(now time.Time) ([]domain.Workload, error) {
	var completed []domain.Workload
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		completed, err = state.CompleteExpiredWorkloads(now)
		return err
	}, nil)
	return completed, err
}

func (s *PostgresStore) ScheduleWorkload(id string, now time.Time) (SchedulingResult, error) {
	var result SchedulingResult
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		result, err = state.ScheduleWorkload(id, now)
		return err
	}, nil)
	return result, err
}

func (s *PostgresStore) SchedulePendingWorkloads(now time.Time) ([]SchedulingResult, error) {
	var results []SchedulingResult
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		results, err = state.SchedulePendingWorkloads(now)
		return err
	}, nil)
	return results, err
}

func (s *PostgresStore) FailNode(id string, now time.Time) (DisruptionResult, error) {
	var result DisruptionResult
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		result, err = state.FailNode(id, now)
		return err
	}, nil)
	return result, err
}

func (s *PostgresStore) RecoverNode(id string, now time.Time) (DisruptionResult, error) {
	var result DisruptionResult
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		result, err = state.RecoverNode(id, now)
		return err
	}, nil)
	return result, err
}

func (s *PostgresStore) PreemptSpotNode(id string, now time.Time) (DisruptionResult, error) {
	var result DisruptionResult
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		result, err = state.PreemptSpotNode(id, now)
		return err
	}, nil)
	return result, err
}

func (s *PostgresStore) CreateNode(node domain.Node) (domain.Node, error) {
	var created domain.Node
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		created, err = state.CreateNode(node)
		return err
	}, nil)
	return created, err
}

func (s *PostgresStore) GetNode(id string) (domain.Node, bool) {
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, gpu_type, total_gpus, allocated_gpus, region, data_center, zone, provider, capacity_class, health, running_workload_ids, created_at, updated_at
		FROM nodes
		WHERE id = $1
	`, id)
	if err != nil {
		return domain.Node{}, false
	}
	defer rows.Close()
	nodes, err := scanNodes(rows)
	if err != nil || len(nodes) == 0 {
		return domain.Node{}, false
	}
	return nodes[0], true
}

func (s *PostgresStore) ListNodes() []domain.Node {
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, gpu_type, total_gpus, allocated_gpus, region, data_center, zone, provider, capacity_class, health, running_workload_ids, created_at, updated_at
		FROM nodes
		ORDER BY id
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	nodes, err := scanNodes(rows)
	if err != nil {
		return nil
	}
	return nodes
}

func (s *PostgresStore) UpdateNode(id string, fn func(*domain.Node) error) (domain.Node, error) {
	var updated domain.Node
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		updated, err = state.UpdateNode(id, fn)
		return err
	}, nil)
	return updated, err
}

func (s *PostgresStore) CreateEvent(event domain.Event) (domain.Event, error) {
	var created domain.Event
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		created, err = state.CreateEvent(event)
		return err
	}, nil)
	return created, err
}

func (s *PostgresStore) GetEvent(id string) (domain.Event, bool) {
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, type, actor, workload_id, node_id, message, metadata
		FROM events
		WHERE id = $1
	`, id)
	if err != nil {
		return domain.Event{}, false
	}
	defer rows.Close()
	events, err := scanEvents(rows)
	if err != nil || len(events) == 0 {
		return domain.Event{}, false
	}
	return events[0], true
}

func (s *PostgresStore) ListEvents() []domain.Event {
	ctx := context.Background()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, type, actor, workload_id, node_id, message, metadata
		FROM events
		ORDER BY timestamp DESC, id DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	events, err := scanEvents(rows)
	if err != nil {
		return nil
	}
	return events
}

func (s *PostgresStore) UpdateEvent(id string, fn func(*domain.Event) error) (domain.Event, error) {
	var updated domain.Event
	err := s.withState(context.Background(), func(state *MemoryStore) error {
		var err error
		updated, err = state.UpdateEvent(id, fn)
		return err
	}, nil)
	return updated, err
}

func (s *PostgresStore) countSummary(ctx context.Context) (DemoDataSummary, error) {
	var summary DemoDataSummary
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM nodes),
			(SELECT COUNT(*) FROM workloads),
			(SELECT COUNT(*) FROM events)
	`).Scan(&summary.Nodes, &summary.Workloads, &summary.Events); err != nil {
		return DemoDataSummary{}, err
	}
	return summary, nil
}

func (s *PostgresStore) replaceAll(ctx context.Context, state *MemoryStore) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)

	if err := truncateAll(ctx, tx); err != nil {
		return err
	}
	if err := insertState(ctx, tx, state); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) withState(ctx context.Context, fn func(*MemoryStore) error, after func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)

	state, err := loadState(ctx, tx)
	if err != nil {
		return err
	}
	if err := fn(state); err != nil {
		return err
	}
	if err := truncateAll(ctx, tx); err != nil {
		return err
	}
	if err := insertState(ctx, tx, state); err != nil {
		return err
	}
	if after != nil {
		if err := after(tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadState(ctx context.Context, tx *sql.Tx) (*MemoryStore, error) {
	state := NewMemoryStore()

	nodes, err := scanNodesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		state.nodes[node.ID] = cloneNode(node)
	}

	workloads, err := scanWorkloadsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, workload := range workloads {
		state.workloads[workload.ID] = cloneWorkload(workload)
	}

	events, err := scanEventsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		state.events[event.ID] = cloneEvent(event)
	}

	return state, nil
}

func truncateAll(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		`DELETE FROM events`,
		`DELETE FROM workloads`,
		`DELETE FROM nodes`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func insertState(ctx context.Context, tx *sql.Tx, state *MemoryStore) error {
	for _, node := range state.ListNodes() {
		runningIDs, err := json.Marshal(node.RunningWorkloadIDs)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO nodes (
				id, gpu_type, total_gpus, allocated_gpus, region, data_center, zone, provider, capacity_class, health, running_workload_ids, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		`, node.ID, node.GPUType, node.TotalGPUs, node.AllocatedGPUs, node.Region, node.DataCenter, node.Zone, node.Provider, string(node.CapacityClass), string(node.Health), runningIDs, node.CreatedAt, node.UpdatedAt); err != nil {
			return err
		}
	}

	for _, workload := range state.ListWorkloads() {
		replicas := workload.Replicas
		if replicas < 1 {
			replicas = 1
		}
		var placementJSON []byte
		if workload.Placement != nil {
			var err error
			placementJSON, err = json.Marshal(workload.Placement)
			if err != nil {
				return err
			}
		}
		replicaPlacementsJSON := []byte("[]")
		if len(workload.ReplicaPlacements) > 0 {
			var err error
			replicaPlacementsJSON, err = json.Marshal(workload.ReplicaPlacements)
			if err != nil {
				return err
			}
		}
		var drainStartedAt any
		if workload.DrainStartedAt != nil {
			drainStartedAt = *workload.DrainStartedAt
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workloads (
				id, type, gpu_type, gpu_count, priority, duration_seconds, spot_tolerant, resumable, replicas, replica_placements, state, placement, status_reason, scheduling_explanation, preempt_notice_seconds, drain_started_at, checkpoint_state, resume_eligible, submitted_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		`, workload.ID, string(workload.Type), workload.GPUType, workload.GPUCount, string(workload.Priority), workload.DurationSeconds, workload.SpotTolerant, workload.Resumable, replicas, replicaPlacementsJSON, string(workload.State), placementJSON, workload.StatusReason, workload.SchedulingExplanation, workload.PreemptNoticeSeconds, drainStartedAt, workload.CheckpointState, workload.ResumeEligible, workload.SubmittedAt, workload.UpdatedAt); err != nil {
			return err
		}
	}

	for _, event := range state.ListEvents() {
		var metadataJSON []byte
		if event.Metadata != nil {
			var err error
			metadataJSON, err = json.Marshal(event.Metadata)
			if err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO events (
				id, timestamp, type, actor, workload_id, node_id, message, metadata
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, event.ID, event.Timestamp, event.Type, event.Actor, event.WorkloadID, event.NodeID, event.Message, metadataJSON); err != nil {
			return err
		}
	}
	return nil
}

func scanNodesTx(ctx context.Context, tx *sql.Tx) ([]domain.Node, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, gpu_type, total_gpus, allocated_gpus, region, data_center, zone, provider, capacity_class, health, running_workload_ids, created_at, updated_at
		FROM nodes
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

func scanWorkloadsTx(ctx context.Context, tx *sql.Tx) ([]domain.Workload, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, type, gpu_type, gpu_count, priority, duration_seconds, spot_tolerant, resumable, replicas, replica_placements, state, placement, status_reason, scheduling_explanation, preempt_notice_seconds, drain_started_at, checkpoint_state, resume_eligible, submitted_at, updated_at
		FROM workloads
		ORDER BY submitted_at, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWorkloads(rows)
}

func scanEventsTx(ctx context.Context, tx *sql.Tx) ([]domain.Event, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, timestamp, type, actor, workload_id, node_id, message, metadata
		FROM events
		ORDER BY timestamp, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanNodes(rows *sql.Rows) ([]domain.Node, error) {
	nodes := make([]domain.Node, 0)
	for rows.Next() {
		var node domain.Node
		var runningIDs []byte
		if err := rows.Scan(&node.ID, &node.GPUType, &node.TotalGPUs, &node.AllocatedGPUs, &node.Region, &node.DataCenter, &node.Zone, &node.Provider, &node.CapacityClass, &node.Health, &runningIDs, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, err
		}
		if len(runningIDs) > 0 {
			if err := json.Unmarshal(runningIDs, &node.RunningWorkloadIDs); err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, cloneNode(node))
	}
	return nodes, rows.Err()
}

func scanWorkloads(rows *sql.Rows) ([]domain.Workload, error) {
	workloads := make([]domain.Workload, 0)
	for rows.Next() {
		var workload domain.Workload
		var placementJSON []byte
		var replicaPlacementsJSON []byte
		var drainStartedAt sql.NullTime
		if err := rows.Scan(&workload.ID, &workload.Type, &workload.GPUType, &workload.GPUCount, &workload.Priority, &workload.DurationSeconds, &workload.SpotTolerant, &workload.Resumable, &workload.Replicas, &replicaPlacementsJSON, &workload.State, &placementJSON, &workload.StatusReason, &workload.SchedulingExplanation, &workload.PreemptNoticeSeconds, &drainStartedAt, &workload.CheckpointState, &workload.ResumeEligible, &workload.SubmittedAt, &workload.UpdatedAt); err != nil {
			return nil, err
		}
		if len(replicaPlacementsJSON) > 0 {
			if err := json.Unmarshal(replicaPlacementsJSON, &workload.ReplicaPlacements); err != nil {
				return nil, err
			}
		}
		if len(placementJSON) > 0 {
			var placement domain.Placement
			if err := json.Unmarshal(placementJSON, &placement); err != nil {
				return nil, err
			}
			workload.Placement = &placement
		}
		if drainStartedAt.Valid {
			value := drainStartedAt.Time
			workload.DrainStartedAt = &value
		}
		if workload.Replicas < 1 {
			workload.Replicas = 1
		}
		workloads = append(workloads, cloneWorkload(workload))
	}
	return workloads, rows.Err()
}

func scanEvents(rows *sql.Rows) ([]domain.Event, error) {
	events := make([]domain.Event, 0)
	for rows.Next() {
		var event domain.Event
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.Timestamp, &event.Type, &event.Actor, &event.WorkloadID, &event.NodeID, &event.Message, &metadata); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, err
			}
		}
		events = append(events, cloneEvent(event))
	}
	return events, rows.Err()
}

func rollback(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

func (s *PostgresStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
