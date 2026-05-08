# Video Walkthrough

Video: https://drive.google.com/file/d/1W4ALtSgP8XJlFIabuxQR2JNrWMwI_1m4/view?usp=sharing

Live demo: http://34.105.88.70/#user-view

## Transcript

Hi Luma Labs team,

I am presenting my solution for the take-home test.

I built a web app using React, Vite, and TypeScript. For the backend, I chose Go, and for the demo runtime I used Postgres as the backing database.

The solution targets two kinds of users: an enterprise customer and an internal admin user.

For the scope of this demo, I intentionally left authentication, authorization, rate limiting, pricing, and other enterprise features out of scope. I also vibe-coded the frontend and intentionally left detailed client-side validation out of scope. The project is scoped for a logged-in user with non-malicious intent.

An enterprise user can request a new workload by selecting standard workload properties. They can also monitor submitted workloads. Currently all workloads are visible, but with the right RBAC model this can be extended to teams, members, and tenant-level visibility.

A logged-in admin can open the Admin dashboard to view fleet health, GPU utilization, and workload execution state. The admin can also run simulations to see how the system reacts to events like inference demand spikes, spot preemption, node failures, and exhausted capacity. The event log tells the operational story of how the system responded.

The Admin ops view is meant for operational controls. It lets an admin seed or clear demo data and manually trigger node-level actions for the simulated fleet.

Finally, I added a System design overview section that explains the internals of the system. It includes a high-level architecture diagram, API sequence diagrams for different flows, detailed decision logic for scheduling and preemption, and a limitations section that calls out what I would improve next.

The goal of this project is to demonstrate the control plane: how workloads are accepted, how placement decisions are made, how disruptions are handled, and how operational state is surfaced to users and admins.

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
