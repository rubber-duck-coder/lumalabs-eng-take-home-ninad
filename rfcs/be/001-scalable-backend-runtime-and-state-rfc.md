# Backend RFC 001: Scalable Runtime And State

## Goal

Define the scalable backend direction for scheduler, workload, and fleet services so v1 remains simple but extensible toward high-throughput, globally distributed, highly available operation.

## Non-Goals

- Implement multi-region HA in v1.
- Implement distributed consensus in v1.
- Build real GPU provisioning in v1.
- Add Kubernetes or service mesh in v1.
- Optimize before scheduler correctness is proven.

## New Requirements

- Scheduler, workload, and fleet services should be designed for low latency and high concurrency.
- System should tolerate traffic spikes from enterprise users and internal automation.
- Fleet model should support multiple regions, data centers, zones, and providers.
- Future architecture should survive data center outages, network partitions, grid failures, and regional calamities.
- Initial implementation should preserve service boundaries so runtime and storage can evolve without rewriting domain logic.

## Runtime Evaluation

### Go

Pros:
- Excellent concurrency primitives.
- Low memory overhead and fast startup.
- Strong fit for high-throughput control plane services.
- Simple deployment as static binaries or containers.
- Mature ecosystem for HTTP APIs, workers, metrics, and distributed systems.

Cons:
- Slower initial product iteration than Python for some teams.
- More boilerplate for validation and tests.

Fit:
- Best long-term choice for scheduler, workload, and fleet services.
- Strong latency, availability, and operational profile.

### Python / FastAPI

Pros:
- Fastest initial iteration.
- Strong API ergonomics and test speed.
- Good enough for v1 demo traffic.

Cons:
- Weaker CPU-bound concurrency story.
- More care needed for async/background execution.
- Less ideal for ultra-low-latency scheduler loops.

Fit:
- Good v1 implementation choice if speed matters most.
- Less ideal as the long-term core runtime.

### TypeScript / Node.js

Pros:
- Fast developer iteration.
- Strong frontend/backend sharing potential.
- Large ecosystem.

Cons:
- Event loop requires care for CPU-heavy scheduling.
- Runtime behavior less predictable for control-plane hot paths.

Fit:
- Good gateway/API layer option.
- Weaker fit for scheduler core.

### Java / Kotlin

Pros:
- Very mature enterprise backend ecosystem.
- Strong performance and concurrency.
- Good database and observability tooling.

Cons:
- Heavier runtime and slower project setup.
- More ceremony for take-home scope.

Fit:
- Strong production choice, but too heavy for v1.

## Runtime Decision

Use Go as the target runtime for production-grade scheduler, workload, and fleet services.

For v1, implement one Go process with clear internal modules:
- `gateway`
- `workloads`
- `scheduler`
- `fleet`
- `events`

Keep REST as the external contract. Consider gRPC only later if modules split into independent services.

## State Requirements

### Workload State

- Strong consistency for workload lifecycle transitions.
- Idempotent submissions and admin commands.
- Durable event history for audit/debug.
- Queryable by state, priority, tenant/team later, region, and placement.

### Fleet State

- Frequent updates from many nodes/data centers.
- Region, data center, zone, provider, capacity class, and health metadata.
- Need fast reads for scheduler decisions.
- Must tolerate stale or partial data during partitions.

### Scheduler State

- Needs consistent view of pending workloads and allocatable capacity.
- Must avoid double allocation.
- Should support leader election or partitioned scheduling later.
- Must emit explainable decisions.

## Database Evaluation

### Postgres

Pros:
- Strong consistency and transactions.
- Good fit for workload lifecycle and allocation correctness.
- Mature managed SaaS support.
- Query flexibility for dashboards and audits.
- Can model event log, workload state, and fleet inventory initially.

Cons:
- Single-region primary can become a latency/availability bottleneck.
- Multi-region active-active is complex.

Fit:
- Best default for v1 and near-term production.
- Strong choice for correctness-first control plane state.

### DynamoDB / Cloud NoSQL

Pros:
- High availability and scale.
- Global tables support multi-region patterns.
- Good for high-volume fleet heartbeats/events.

Cons:
- Query modeling is stricter.
- Transactions and relational queries are less ergonomic.
- Local development and portability are worse.

Fit:
- Strong future option for globally distributed fleet/event state.
- Less ideal for v1 portability and reviewer setup.

### CockroachDB / Distributed SQL

Pros:
- SQL with multi-region replication.
- Better regional survivability story than single-primary Postgres.
- Strong fit for globally distributed control planes.

Cons:
- Operationally heavier.
- More complexity than needed for take-home.

Fit:
- Strong future HA option if global SQL consistency becomes central.

### Redis

Pros:
- Very fast.
- Useful for queues, locks, cache, and rate limiting.

Cons:
- Not primary durable state by default.
- HA durability requires careful setup.

Fit:
- Good supporting component later.
- Not primary source of truth.

## State Decision

Use Postgres as the target source of truth for workload, fleet, scheduler, and event state.

For v1:
- In-memory state is acceptable only for the first walking skeleton.
- Prefer Postgres once API integration and E2E tests start.
- Use repository interfaces so storage can evolve toward distributed SQL or cloud NoSQL later.

Why not SQLite:
- Weak fit for concurrent writes.
- Poor fit for deployed multi-instance services.
- Not aligned with HA or multi-data-center requirements.

## HA Direction

Target future shape:

- Gateway API is stateless and horizontally scalable.
- Scheduler Service runs with leader election per scheduling partition.
- Workload Service owns lifecycle state in Postgres.
- Fleet Service ingests regional node health and capacity updates.
- Event Service stores append-only audit/debug history.
- Regional services continue operating during partial outages.
- Global control plane degrades gracefully when a region or data center is unavailable.

Initial extensibility requirements:

- Include `region`, `data_center`, `zone`, and `provider` in fleet model.
- Keep service boundaries explicit even inside one process.
- Make all write APIs idempotent where practical.
- Keep scheduler deterministic and side-effect boundaries narrow.
- Treat events as first-class records, not logs only.

## Impact On Existing Backend RFC

- Replace FastAPI as long-term backend direction with Go for core services.
- Keep REST API contracts unless a later RFC justifies gRPC internally.
- Replace SQLite consideration with Postgres as target durable state.
- Keep v1 simple, but avoid design choices that block service split or Postgres adoption.

## Open Questions

- Should Postgres be local-only for v1 via Docker Compose, or also used in the hosted demo?
- Should scheduler be single global leader first, or partitioned by region/GPU type later?
- Do we want API-level multi-tenancy fields now, even without auth?

## Decision Log

- Decision: use Go for v1 Gateway API and core backend modules.
- Rationale: one backend language keeps tooling, deployment, observability, and ownership simple while aligning with low-latency, concurrency-heavy control-plane goals.
- Decision: implement v1 as a modular monolith, not separate services.
- Rationale: preserves future service boundaries without adding distributed-system overhead before the product behavior is proven.
- Decision: use REST externally and defer gRPC internally.
- Rationale: REST is easier for frontend, E2E, and reviewer interaction; gRPC becomes useful only after service separation.
- Decision: use Postgres for the demo/runtime database and CockroachDB for production multi-region durability.
- Rationale: Postgres is the easiest operational fit for the take-home demo, while CockroachDB better matches the production requirement for native multi-region support.
