# Frontend RFC

## Goal

Build a minimal React/Vite/TypeScript UI that makes the control plane easy to evaluate: submit workloads, inspect scheduling outcomes, view fleet health/utilization, and trigger disruptions.

## Non-Goals

- Auth, roles, tenants.
- Client-side scheduling logic.
- Billing or quotas.
- Complex charting or design system.

## User Surfaces

- Enterprise: submit workload, view lifecycle state, placement, queue reason, and explanation.
- Admin: view fleet inventory, utilization, workloads, events, and disruption controls.

## Initial UI Shape

- Start as one dashboard page with sections.
- Split into routes only if the page becomes hard to scan.
- Manual refresh is acceptable first; polling can be added later.

## Core Components

- `WorkloadForm`
- `WorkloadTable`
- `WorkloadDetail`
- `FleetInventory`
- `UtilizationSummary`
- `EventTimeline`
- `DisruptionControls`

## Backend Contract Needs

- `GET /health`
- `POST /workloads`
- `GET /workloads`
- `GET /workloads/{id}`
- `GET /nodes`
- `GET /fleet/summary`
- `GET /events`
- `POST /scheduler/tick`
- `POST /admin/nodes/{id}/fail`
- `POST /admin/nodes/{id}/recover`
- `POST /admin/nodes/{id}/preempt-spot`
- `POST /admin/workloads/{id}/preempt` with checkpoint/drain metadata when the backend exposes it

## Explanation UX

Backend responses should include structured decision data:

- `decision`: placed, queued, rescheduled, preempted, failed, completed.
- `reason`: concise human-readable reason.
- `selected_node_id`: present when placed.
- `rejected_nodes`: concise rejection reasons when queued.
- `event_ids`: links to related events.
- `preempt_notice_seconds`, `drain_started_at`, and checkpoint state for workloads that can survive disruption.

UI should show short explanations inline and details on demand.

## Testing

- Unit/component tests for form validation, API parsing, loading/error states, and explanation rendering.
- E2E path uses real backend and covers submit, inspect, disrupt, and observe recovery/queueing.
- Same E2E suite must run against local and deployed `BASE_URL`.

## Phases

- Phase 1: single-page dashboard, submit form, workload list, fleet summary, recent events.
- Phase 2: explanation detail, disruption controls, affected workload highlighting.
- Phase 3: deployed API config, polish, E2E validation.

## Open Questions

- Is one page sufficient for the final demo?
- Should UI refresh be manual or light polling?
- What exact explanation schema will backend expose?

## Technology Decisions

### UI Framework: React + Vite + TypeScript

Decision: use React/Vite/TypeScript.

Pros:
- Fast local setup and static build.
- Familiar component model for dashboard UI.
- TypeScript helps stabilize API contracts.
- Easy to deploy as static assets or serve from backend.

Cons:
- More setup than server-rendered HTML.
- Adds frontend build tooling.

Alternatives:
- Plain HTML/templates: lowest complexity, but weaker interactivity and component testing.
- Next.js: strong full-stack framework, but unnecessary routing/server complexity.

NFR fit:
- Latency: static assets plus REST calls are sufficient for demo.
- Availability: can be served by same app to reduce moving parts.
- Maintainability: TypeScript contracts reduce integration drift.

### Refresh Model: Manual First

Decision: start with manual refresh; add light polling only if needed.

Pros:
- Deterministic and test-friendly.
- Avoids websocket/SSE complexity.

Cons:
- Less real-time feel.

NFR fit:
- Reliability matters more than live updates for this take-home.
- Lower complexity improves delivery speed.
