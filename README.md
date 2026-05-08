# Luma Take-Home — Infrastructure Engineering

GPU compute is the backbone of modern AI companies. Training runs, inference serving, batch processing — they all compete for the same scarce, expensive hardware. The difference between a well-run GPU fleet and a poorly-run one is millions of dollars and weeks of wasted researcher time.

The hardest part isn't getting one job running on one GPU. It's everything else: scheduling across heterogeneous hardware, surviving preemption, shifting capacity between training and inference as priorities change, and keeping utilization high while maintaining SLOs.

**Build the control plane for a GPU compute platform.** You have ~1 working day.

**You must use AI coding tools** — Claude Code, Cursor, Codex, whatever you prefer. These problems are scoped so that AI is necessary to ship something real in a day. We want to see how you direct the tools: how you plan, how you course-correct, what you accept, and what you push back on.

---

## The Problem

Build a system that manages GPU workloads across a fleet of machines. The fleet is heterogeneous — different GPU types (think A100s, H100s, L4s), a mix of on-demand and spot/preemptible capacity, multiple availability zones or providers. Workloads vary: training jobs are long-running and need reliable capacity; inference services need low latency and horizontal scaling; batch jobs are flexible on timing and optimize for cost.

Your system should accept workloads, decide where to place them, and react when things change — a spot instance gets preempted, a higher-priority job needs capacity, a node goes unhealthy, demand shifts from training to inference.

The GPU nodes themselves can be simulated — what matters is how your control plane makes decisions, handles disruptions, and surfaces operational state. We want to see a real system that runs, makes real scheduling decisions, and demonstrates how it behaves when things go wrong.

This is deliberately open-ended. We want to see which parts of the problem you choose to tackle and why.

---

## Tips

The candidates who do best don't start by building — they start by getting sharp on the problem. It's easy to either throw everything at the wall or get heads-down on making something work, and miss the more important question: *what's actually worth solving here, and for whom?*

Slow down before you write a line of code. The thinking you do upfront will shape everything.

---

## What We're Looking For

We want to see infrastructure thinking applied to a real operational problem — not a design doc, not a YAML templating exercise. A system that demonstrates a real opinion about how GPU workloads should be managed — where the hard tradeoffs are and how you'd navigate them.

We expect the result to be better than what an AI would produce on its own with minimal guidance. Your judgment about scheduling policies, failure modes, operational ergonomics, and what actually matters at scale — that's what we're evaluating. Specifically, we're paying attention to:

- **How you approach new problems** — how you break down ambiguity, decide what to tackle first, and make good decisions with incomplete information
- **How you use AI tools** — not just that you used them, but how you directed them, where you pushed back, and where your judgment shaped the result
- **The unique perspective you bring** — the infrastructure instincts, systems experience, or architectural taste that made your solution distinct from what anyone else would have built

---

## What to Deliver

### 1. Working software

Build your solution directly in this repo. It should run. We should be able to submit workloads, see scheduling decisions, trigger disruptions (preemption, node failures), and observe how your system responds. Include setup instructions that work in a fresh Linux environment — we will run your system during review.

Docker is strongly encouraged. A `docker-compose.yml` that brings up the full stack is ideal.

**If your project is deployable, deploy it.** A live URL, a dashboard, or a recorded demo of the system in action goes a long way. Include it in your APPROACH.md.

Deployment runbook for the Google Cloud VM path: [docs/google-cloud-vm.md](docs/google-cloud-vm.md).

### Local Development And GCP Deployment Path

This project was built iteratively in local Docker (`docker compose`) and then deployed to a Google Cloud VM for live verification.

Local developer flow:

```bash
cp .env.example .env
make compose-up
make verify
BASE_URL=http://localhost:5173 make e2e
```

GCP reviewer flow (requires `gcloud`, plus `GOOGLE_CLOUD_PROJECT` and a service-account JSON path):

```bash
export GOOGLE_CLOUD_PROJECT="<your-project-id>"
export GCP_CREDENTIALS_FILE="<path-to-service-account-json>"

# Optional overrides
export GCP_VM_NAME="luma-take-home-review"
export GCP_ZONE="us-west1-b"

# One command to create VM, deploy the stack, and print shareable URL
make gcp-vm-reviewer
```

If you already use the default Google credentials environment variable locally, point `GCP_CREDENTIALS_FILE` at the same JSON key path.

What `make gcp-vm-reviewer` does:
- Authenticates and sets gcloud project.
- Creates a VM if it does not already exist.
- SSHes into the VM, installs required tools (`git`, `make`, Docker), clones/pulls this repo, and runs `docker compose up --build -d`.
- Prints a shareable URL like `http://<EXTERNAL_IP>` for team review.

Additional helper commands:

```bash
make gcp-vm-create   # create/reuse VM
make gcp-vm-ssh      # interactive SSH session
make gcp-vm-deploy   # re-deploy latest main on VM
make gcp-vm-url      # print external URL + health check
```

A `.env.example` is included with stub keys for providers we have accounts with (Anthropic, OpenAI, Google Cloud, AWS). Copy it to `.env`, use whichever keys your solution needs, and document any others.

### 2. APPROACH.md

- What you built and why — which parts of the orchestration problem did you focus on?
- Architecture decisions and tradeoffs — scheduling algorithm, state management, failure handling
- How you modeled the fleet and workloads — what assumptions did you make?
- What you intentionally left out
- What breaks first under pressure
- What you'd build next with more time

### 3. Video walkthrough

Record a short video (~5 minutes) showing your system end-to-end. Submit workloads, show scheduling decisions being made, trigger a disruption, and walk through how your system responds. Explain the architecture and the tradeoffs you made. We want to see how you think about the problem, not just what you built.

**Paste your video link (Loom, Google Drive, YouTube, etc.) into `video.md`.**

### 4. AI session history

Your AI session logs (Claude Code, Codex, Cursor) are packaged automatically when you run `./submit.sh`. If you used other AI tools (ChatGPT, etc.), export those conversations and include them in your repo before submitting.

This is a required deliverable. We review your AI interaction to understand how you work — how you plan, iterate, and direct the tools.

---

## Submitting

When you're ready, run the submit script from your repo root:

```bash
./submit.sh
```

This packages your AI session history, commits and pushes your latest changes, grants reviewer access, and registers your submission. You'll see a confirmation when it's done.

---

## Logistics

### Costs & API keys
We'll reimburse up to $100 total. Don't commit secrets — use `.env`.

### Time
~8 hours. No strict deadline, but typically within 2-3 days.

### Questions
Ask anytime — treat this like working with a teammate.
