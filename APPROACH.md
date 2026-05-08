# Approach

## What I Built

I built a runnable GPU workload control plane with a Go backend, Postgres persistence, Docker Compose runtime, and a React admin UI. The system accepts workloads, schedules them onto a simulated heterogeneous GPU fleet, reacts to disruptions, records operational events, and exposes telemetry for fleet and workload state.

The demo focuses on control-plane behavior rather than GPU execution. The GPU nodes are simulated, but scheduling, queueing, preemption, node failure, recovery, reconciliation, eventing, and telemetry all execute through real API and persistence paths.

Live demo: http://34.105.88.70/#user-view

Video walkthrough: https://drive.google.com/file/d/1W4ALtSgP8XJlFIabuxQR2JNrWMwI_1m4/view?usp=sharing

## How To Run

```bash
cp .env.example .env
make compose-up
```

Open:

```text
http://localhost:5173
```

Verify locally:

```bash
make verify
BASE_URL=http://localhost:5173 make e2e
```

For a Google Cloud VM deployment, use:

```bash
export GOOGLE_CLOUD_PROJECT="<project-id>"
export GCP_CREDENTIALS_FILE="<path-to-service-account-json>"
make gcp-vm-reviewer
```

## Architecture

The backend is a modular monolith:

- `gateway`: HTTP API, validation, response shaping.
- `controlplane`: orchestration across workloads, fleet, events, telemetry, and simulations.
- `scheduler`: deterministic placement policy and queue explanations.
- `workloads`: submit, schedule, queue, complete, and pending tick flows.
- `fleet`: node health, recovery, failure, and spot preemption flows.
- `reconciler`: background completion/recovery loop.
- `events`: append-only operational audit events.
- `telemetry`: fleet/workload snapshots for dashboard charts.
- `store`: in-memory and Postgres-backed state adapters.

The frontend is a React/Vite dashboard with four primary views:

- `User view`: submit workloads and monitor placement.
- `Admin dashboard`: fleet health, telemetry, simulations, and event log.
- `Admin ops`: seed/clear demo data and trigger node-level disruption controls.
- `System design overview`: interactive architecture, API sequence, limitations, and decision-logic diagrams.

## Workload And Fleet Model

Nodes have:

- GPU type: `A100`, `H100`, `L4`.
- Capacity class: `on_demand` or `spot`.
- Health: `healthy`, `recovering`, or `failed`.
- Region/provider/zone metadata.
- Total and allocated GPU counts.

Workloads have:

- Type: `training`, `inference`, or `batch`.
- GPU type and GPU count.
- Priority: `low`, `normal`, or `high`.
- Duration for demo completion.
- Spot tolerance.
- Resumability/checkpoint metadata.
- Replicas for inference scale-out.

## Scheduling Strategy

Scheduling uses hard constraints first, then class-aware scoring.

Hard constraints:

- GPU type must match.
- Node must have enough free GPUs.
- Node must be healthy.
- Capacity class must be compatible with workload policy.

Policy order:

```text
priority first, then inference > training > batch
on-demand > spot unless the workload is explicitly spot-tolerant and the class benefits from spot
```

Class behavior:

- `training`: prefers stable on-demand capacity and tight packing to reduce fragmentation.
- `inference`: prefers on-demand capacity and lower-utilized nodes; replicas spread across eligible nodes where possible.
- `batch`: prefers spot when spot-tolerant, otherwise uses on-demand and packs tightly.

Preemption is intentionally conservative. A higher-priority workload can reclaim capacity from lower-priority running work only when that creates a valid fit. Affected workloads receive explicit state transitions, reasons, and events. Resumability/checkpoint metadata is modeled, but actual checkpoint execution is outside this simulated control plane.

## Failure Handling

The system supports:

- Node failure.
- Node recovery.
- Spot preemption.
- Scheduler tick.
- Background reconciliation.
- Workload completion based on duration.
- One-click simulations:
  - sudden inference spike,
  - spot preemption,
  - node failures,
  - capacity exhaustion.

Disruptions requeue or reschedule affected workloads and emit events so the dashboard tells a clear operational story.

## Persistence

The demo path uses Postgres via Docker Compose. I started with an in-memory store for fast iteration, then moved the runtime path to Postgres so state survives API restarts and the reviewer can inspect real backing state.

For production, I would prefer CockroachDB or another strongly consistent multi-region datastore because GPU control-plane state benefits from native multi-region availability and transactional correctness across regions.

## Observability

The API exposes:

- workload list and placement reasons,
- node inventory and health,
- fleet summary,
- event log,
- telemetry snapshots.

The dashboard shows fleet utilization, GPU availability, node health, workload state trends, simulation outcomes, and collapsible event metadata.

## What I Intentionally Left Out

- Real GPU runtime integration.
- Real checkpoint execution.
- Kubernetes or cloud-provider node discovery.
- Authentication/authorization.
- Multi-region active-active control plane.
- Production-grade scheduler sharding.
- Prometheus/OpenTelemetry integration.
- Production migration tooling.

These were deferred to keep the demo focused on control-plane decision making and failure behavior.

## What Breaks First Under Pressure

- The Postgres adapter still favors simple state snapshots over row-level, high-throughput transactional updates.
- List APIs are not paginated.
- Scheduler/reconciler loops are centralized and would need sharding or leader election before horizontal scale.
- Event and telemetry schemas are useful for demo analysis but not yet rich enough for production incident correlation.
- Demo/admin endpoints need gating before any real environment.

The System Design `Limitations` tab calls these out directly in the UI with future mitigation paths.

## What I Would Build Next

1. Replace snapshot-style persistence with row-level transactions, optimistic version checks, and an outbox for events.
2. Add authn/authz and separate user/admin capabilities.
3. Add pagination and filtering for workload/event APIs.
4. Make reconciliation event-driven and shard scheduling queues by GPU type/region.
5. Add OpenTelemetry traces and Prometheus metrics.
6. Add performance, chaos, and replay tests for scheduling decisions.
7. Move production state to CockroachDB or managed multi-region Postgres with explicit failover behavior.

## AI Usage

I used AI coding tools as an implementation partner and reviewer. I directed them through phased execution, asked for adversarial reviews when design risk increased, rejected or revised implementation ideas when they did not match the system direction, and used the feedback to harden the scheduler, persistence, frontend UX, deployment flow, and documented limitations.
