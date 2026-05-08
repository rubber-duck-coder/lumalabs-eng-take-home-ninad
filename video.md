# Video Walkthrough

<replace with your Loom link or Google Drive URL>

## Suggested 5 Minute Script

1. Open the dashboard and explain the four views:
   - User view
   - Admin dashboard
   - Admin ops
   - System design overview
2. Seed demo data from Admin ops.
3. Submit a training or inference workload from User view and show placement/queue reason.
4. Run a simulation from Admin dashboard:
   - sudden inference spike,
   - spot preemption,
   - node failures,
   - capacity exhausted.
5. Show fleet telemetry, workload telemetry, and event log after the simulation.
6. Open System design overview:
   - high-level architecture,
   - API sequence,
   - decision logic,
   - limitations.
7. Close with the main tradeoffs:
   - conservative preemption,
   - class-aware scheduling,
   - Postgres for demo persistence,
   - CockroachDB/multi-region state as the production direction.
