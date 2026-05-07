# GPU Control Plane PRD

## Goal

Build a small, runnable GPU workload control plane that demonstrates how workloads are submitted, scheduled onto a heterogeneous simulated GPU fleet, monitored, and recovered from disruptions.

The project should prioritize clear infrastructure judgment over breadth: realistic scheduling tradeoffs, visible operational state, and simple disruption handling.

## Users

- Enterprise user: external customer or team member requesting GPU capacity for ML workloads.
- Internal admin user: platform operator responsible for fleet health, utilization, and workload reliability.

## Functional Requirements

### Enterprise User Experience

- Submit workloads through a simple UI or API.
- Provide workload type, GPU type, GPU count, priority, simulated duration, and spot tolerance.
- See workload lifecycle state: pending, running, completed, failed, or preempted.
- See placement outcome and scheduling explanation.
- See queueing reason when capacity is unavailable.

### Admin User Experience

- View simulated fleet inventory by node, GPU type, zone/provider, capacity class, and health.
- View aggregate GPU utilization.
- Inspect workloads by state.
- View recent scheduling and disruption events.
- Trigger simulated node failure, spot preemption, and recovery.
- Observe how the scheduler reacts after disruptions.
- Identify capacity pressure and scheduling bottlenecks.

### Core Platform Behavior

- Model a heterogeneous GPU fleet.
- Place workloads based on resource fit, priority, workload type, and spot tolerance.
- Queue workloads when no valid placement exists.
- Reschedule disrupted workloads when possible.
- Record enough events to make scheduler decisions explainable.

## Non-Functional Requirements

### Local Development

- Run from a fresh checkout with minimal setup.
- Prefer Docker Compose for the full local stack.
- Include clear setup and run instructions.
- Include seed/demo data for fleet state and workload examples.

### Testing

- Cover scheduler behavior with automated tests.
- Include tests for resource fit, priority ordering, queueing, and disruption recovery.
- Provide an end-to-end test path that can run against local and deployed environments.

### Deployment

- Support deployment to low-cost or free-tier SaaS.
- Stay within the $100 reimbursement cap.
- Do not require a dedicated domain.
- Avoid real GPU infrastructure, Kubernetes, or expensive always-on cloud resources.
- Prefer simple app hosting such as Render, Fly.io, Railway, or equivalent.
- Use managed persistence only if it materially improves the demo.

### Operability

- Expose current fleet state, workload state, utilization, and recent events.
- Keep failure modes visible in the UI/API.
- Optimize for a reviewer understanding system behavior quickly.

## Out of Scope

- Real GPU provisioning.
- Kubernetes integration.
- Authentication and multi-tenant authorization.
- Billing, quotas, and chargeback.
- Real model serving or training execution.
- Dedicated domain setup.
- Production-grade HA database or distributed scheduler.

## Workload Model

Workloads have:
- `type`: training, inference, or batch.
- `gpu_type`: preferred GPU class such as H100, A100, or L4.
- `gpu_count`: number of GPUs required.
- `priority`: low, normal, or high.
- `duration`: simulated runtime.
- `spot_tolerant`: whether the workload may run on preemptible capacity.

Scheduling intent:
- Training prefers reliable on-demand GPUs.
- Inference prefers available capacity and stable placement.
- Batch prefers cheaper spot capacity when allowed.
- Higher-priority workloads should be scheduled before lower-priority workloads.

## Workstream Boundaries

- Frontend owns enterprise submission UI and admin dashboard.
- Backend owns workload model, fleet model, scheduler, disruption handling, and APIs.
- Infra owns local Docker setup, deployment configuration, environment docs, and release path.

## Success Criteria

- Enterprise user can submit workloads and understand outcomes.
- Admin user can inspect fleet health, utilization, workload state, and disruptions.
- Scheduler demonstrates clear placement, queueing, and recovery decisions.
- Reviewer can run the system locally and against the deployed version.
- The same end-to-end test suite can validate both local setup and production deployment.
- Deployment path is documented and stays within budget constraints.
