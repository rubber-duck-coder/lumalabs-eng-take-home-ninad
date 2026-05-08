import { FormEvent, useEffect, useMemo, useState } from "react";

type Workload = {
  id: string;
  type: string;
  gpu_type: string;
  gpu_count: number;
  priority: string;
  resumable?: boolean;
  replicas?: number;
  replica_placements?: Array<{
    node_id: string;
    region?: string;
    data_center?: string;
    zone?: string;
    provider?: string;
  }>;
  state: string;
  status_reason?: string;
  scheduling_explanation?: string;
  preempt_notice_seconds?: number;
  drain_started_at?: string;
  checkpoint_state?: string;
  resume_eligible?: boolean;
  placement?: {
    node_id: string;
    region?: string;
    data_center?: string;
    zone?: string;
    provider?: string;
  };
};

type Node = {
  id: string;
  gpu_type: string;
  total_gpus: number;
  allocated_gpus: number;
  region: string;
  data_center: string;
  zone: string;
  provider: string;
  capacity_class: string;
  health: string;
  running_workload_ids?: string[];
};

type Event = {
  id: string;
  timestamp: string;
  type: string;
  actor: string;
  workload_id?: string;
  node_id?: string;
  message: string;
  metadata?: Record<string, string>;
};

type FleetSummary = {
  total_gpus: number;
  allocated_gpus: number;
  available_gpus: number;
  utilization_percent: number;
  gpu_types?: Record<string, { total: number; allocated: number }>;
  workloads_by_state?: Record<string, number>;
};

type NodeAction = "fail" | "recover" | "preempt-spot";
type ViewKey = "user-view" | "admin-dashboard" | "admin-ops";

type DisruptionResult = {
  node?: Node;
  Node?: Node;
  affected_workloads?: Workload[];
  scheduled?: Array<{ workload: Workload; decision?: { reason?: string } }>;
};

type DemoDataResult = {
  action: "seed" | "clear";
  nodes: number;
  workloads: number;
  events: number;
};

const apiBase = import.meta.env.VITE_API_BASE_URL || "http://localhost:8080";

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, init);
  if (!response.ok) {
    let detail = "";
    try {
      const payload = (await response.json()) as { error?: string };
      detail = payload.error || "";
    } catch {
      detail = "";
    }
    throw new Error(detail ? `${response.status}: ${detail}` : `request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

function formatError(reason: unknown) {
  return reason instanceof Error ? reason.message : String(reason);
}

function formatTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function clampCount(value: number) {
  return Number.isFinite(value) ? Math.max(0, value) : 0;
}

function tone(value: string) {
  const normalized = value.toLowerCase();
  if (["running", "healthy", "completed"].includes(normalized)) return "success";
  if (["pending", "recovering", "queued"].includes(normalized)) return "warning";
  if (["failed", "preempted", "rejected"].includes(normalized)) return "danger";
  return "neutral";
}

function eventTone(value: string) {
  const normalized = value.toLowerCase();
  if (normalized.includes("fail") || normalized.includes("preempt")) return "danger";
  if (normalized.includes("recover") || normalized.includes("submit")) return "success";
  if (normalized.includes("tick") || normalized.includes("schedule")) return "warning";
  return "neutral";
}

export function App() {
  const [workloads, setWorkloads] = useState<Workload[]>([]);
  const [summary, setSummary] = useState<FleetSummary | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [result, setResult] = useState<Workload | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");
  const [workloadType, setWorkloadType] = useState<Workload["type"]>("training");
  const [error, setError] = useState<string>("");
  const [statusMessage, setStatusMessage] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [adminAction, setAdminAction] = useState<NodeAction | null>(null);
  const [demoAction, setDemoAction] = useState<"seed" | "clear" | null>(null);
  const [tickLoading, setTickLoading] = useState(false);
  const [activeView, setActiveView] = useState<ViewKey>("user-view");

  async function refreshAll() {
    setLoading(true);
    const [workloadsResult, summaryResult, nodesResult, eventsResult] = await Promise.allSettled([
      requestJSON<Workload[]>("/workloads"),
      requestJSON<FleetSummary>("/fleet/summary"),
      requestJSON<Node[]>("/nodes"),
      requestJSON<Event[]>("/events")
    ]);

    const errors: string[] = [];

    if (workloadsResult.status === "fulfilled") {
      setWorkloads(workloadsResult.value);
    } else {
      errors.push(`workloads: ${formatError(workloadsResult.reason)}`);
    }

    if (summaryResult.status === "fulfilled") {
      setSummary(summaryResult.value);
    } else {
      errors.push(`summary: ${formatError(summaryResult.reason)}`);
    }

    if (nodesResult.status === "fulfilled") {
      setNodes(nodesResult.value);
      setSelectedNodeId((current) => {
        if (current && nodesResult.value.some((node) => node.id === current)) {
          return current;
        }
        return nodesResult.value[0]?.id || "";
      });
    } else {
      errors.push(`nodes: ${formatError(nodesResult.reason)}`);
    }

    if (eventsResult.status === "fulfilled") {
      setEvents(eventsResult.value.slice(0, 20));
    } else {
      errors.push(`events: ${formatError(eventsResult.reason)}`);
    }

    setError(errors.join(" | "));
    setLoading(false);
  }

  useEffect(() => {
    refreshAll().catch((err) => setError(formatError(err)));
  }, []);

  useEffect(() => {
    const syncFromHash = () => {
      const hash = window.location.hash.replace("#", "") as ViewKey;
      if (hash === "user-view" || hash === "admin-dashboard" || hash === "admin-ops") {
        setActiveView(hash);
      }
    };

    syncFromHash();
    window.addEventListener("hashchange", syncFromHash);
    return () => window.removeEventListener("hashchange", syncFromHash);
  }, []);

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedNodeId) || null,
    [nodes, selectedNodeId]
  );
  const selectedNodeHealth = selectedNode?.health;
  const selectedNodeCapacityClass = selectedNode?.capacity_class;
  const canFailNode = selectedNodeHealth !== undefined && selectedNodeHealth !== "failed";
  const canRecoverNode = selectedNodeHealth === "failed";
  const canPreemptNode = selectedNodeCapacityClass === "spot";

  const healthyNodes = useMemo(() => nodes.filter((node) => node.health === "healthy"), [nodes]);
  const failedNodes = useMemo(() => nodes.filter((node) => node.health === "failed"), [nodes]);
  const recoveringNodes = useMemo(
    () => nodes.filter((node) => node.health === "recovering"),
    [nodes]
  );
  const activeWorkloads = useMemo(
    () => workloads.filter((workload) => workload.state === "running" || workload.state === "pending"),
    [workloads]
  );
  const workloadSummary = useMemo(
    () => ({
      total: workloads.length,
      active: activeWorkloads.length,
      pending: summary?.workloads_by_state?.pending ?? workloads.filter((w) => w.state === "pending").length,
      running: summary?.workloads_by_state?.running ?? workloads.filter((w) => w.state === "running").length
    }),
    [activeWorkloads.length, summary, workloads]
  );

  const navigationGroups = [
    {
      label: "Workspace",
      items: [
        {
          id: "user-view" as const,
          label: "User view",
          description: "Submit workloads and monitor placement without leaving the page.",
          meta: [`${workloadSummary.total} workloads`, `${workloadSummary.active} active`]
        },
        {
          id: "admin-dashboard" as const,
          label: "Admin dashboard",
          description: "Inspect health, utilization, and the latest system events.",
          meta: [
            `${summary ? summary.utilization_percent.toFixed(1) : "—"}% utilization`,
            `${healthyNodes.length} healthy nodes`
          ]
        }
      ]
    },
    {
      label: "Operations",
      items: [
        {
          id: "admin-ops" as const,
          label: "Admin ops",
          description: "Trigger node disruption, seed, clear, and scheduler actions.",
          meta: [`${failedNodes.length} failed`, `${recoveringNodes.length} recovering`]
        }
      ]
    }
  ];

  const viewMeta: Record<ViewKey, { eyebrow: string; title: string; description: string }> = {
    "user-view": {
      eyebrow: "Page 1",
      title: "Submit workloads and monitor placement",
      description: "Keep the submit path fast, visible, and tied to the live API."
    },
    "admin-dashboard": {
      eyebrow: "Page 2",
      title: "System health, metrics, and event history",
      description: "Fast view of fleet health, utilization, and capacity mix."
    },
    "admin-ops": {
      eyebrow: "Page 3",
      title: "Node disruption controls",
      description: "Choose a node, then apply failure, recovery, or spot preemption."
    }
  };

  function openView(view: ViewKey) {
    setActiveView(view);
    window.history.replaceState(null, "", `#${view}`);
    document.getElementById("page-content")?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setStatusMessage("");
    setSubmitting(true);
    const data = new FormData(event.currentTarget);
    const payload = {
      type: workloadType,
      gpu_type: String(data.get("gpu_type") ?? "A100"),
      gpu_count: Number(data.get("gpu_count")),
      priority: String(data.get("priority") ?? "normal"),
      duration_seconds: Number(data.get("duration_seconds")),
      spot_tolerant: data.get("spot_tolerant") === "on",
      resumable: data.get("resumable") === "on",
      replicas: workloadType === "inference" ? Number(data.get("replicas") ?? 1) : 1
    };

    try {
      const created = await requestJSON<Workload>("/workloads", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });
      setResult(created);
      setStatusMessage(`Submitted ${created.id} and refreshed the live fleet state.`);
      await refreshAll();
      event.currentTarget.reset();
      setWorkloadType("training");
    } catch (err) {
      setError(formatError(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleNodeAction(action: NodeAction) {
    if (!selectedNodeId) {
      setError("Select a node before running an admin action.");
      return;
    }

    setError("");
    setStatusMessage("");
    setAdminAction(action);

    const endpoint = {
      fail: `/admin/nodes/${selectedNodeId}/fail`,
      recover: `/admin/nodes/${selectedNodeId}/recover`,
      "preempt-spot": `/admin/nodes/${selectedNodeId}/preempt-spot`
    }[action];

    try {
      const response = await requestJSON<DisruptionResult>(endpoint, { method: "POST" });
      const node = response.node ?? response.Node;
      if (!node) {
        throw new Error("invalid disruption response: missing node");
      }
      setStatusMessage(
        `${action === "fail" ? "Failed" : action === "recover" ? "Recovered" : "Preempted"} ${node.id}.`
      );
      await refreshAll();
    } catch (err) {
      setError(formatError(err));
    } finally {
      setAdminAction(null);
    }
  }

  async function handleDemoAction(action: "seed" | "clear") {
    setError("");
    setStatusMessage("");
    setDemoAction(action);

    try {
      const response = await requestJSON<DemoDataResult>(`/admin/demo/${action}`, {
        method: "POST"
      });
      setStatusMessage(
        `${response.action === "seed" ? "Seeded" : "Cleared"} demo data: ${response.nodes} nodes, ${response.workloads} workloads, ${response.events} events.`
      );
      await refreshAll();
    } catch (err) {
      setError(formatError(err));
    } finally {
      setDemoAction(null);
    }
  }

  async function handleTick() {
    setError("");
    setStatusMessage("");
    setTickLoading(true);
    try {
      const scheduled = await requestJSON<unknown[]>("/scheduler/tick", { method: "POST" });
      setStatusMessage(`Scheduler tick completed with ${scheduled.length} scheduling result(s).`);
      await refreshAll();
    } catch (err) {
      setError(formatError(err));
    } finally {
      setTickLoading(false);
    }
  }

  const activePage = viewMeta[activeView];

  return (
    <main className="page">
      <header className="hero">
        <div className="hero__eyebrow">Phase 4 admin dashboard</div>
        <div className="hero__top">
          <div>
            <h1>Luma GPU Workload Console</h1>
            <p>
              A page-like control plane UI for workload submission, fleet monitoring, and admin
              operations.
            </p>
          </div>
          <div className="hero__actions">
            <button
              className="button button--secondary"
              onClick={() => refreshAll().catch((err) => setError(formatError(err)))}
              disabled={loading}
            >
              {loading ? "Refreshing..." : "Refresh all"}
            </button>
            <button className="button button--secondary" onClick={handleTick} disabled={tickLoading}>
              {tickLoading ? "Ticking..." : "Run scheduler tick"}
            </button>
          </div>
        </div>

        <div className="hero__meta">
          <span className="meta-pill">Workloads: {workloads.length}</span>
          <span className="meta-pill">Active: {activeWorkloads.length}</span>
          <span className="meta-pill">Nodes: {nodes.length}</span>
          <span className="meta-pill meta-pill--success">Healthy: {healthyNodes.length}</span>
          <span className="meta-pill meta-pill--warning">Recovering: {recoveringNodes.length}</span>
          <span className="meta-pill meta-pill--danger">Failed: {failedNodes.length}</span>
        </div>
      </header>

      {statusMessage && <div className="banner banner--success">{statusMessage}</div>}
      {error && <div className="banner banner--error">{error}</div>}

      <div className="workspace">
        <aside className="sidebar" aria-label="Dashboard navigation and overview">
          <div className="sidebar__panel">
            <span className="sidebar__eyebrow">Overview</span>
            <h2>Live fleet navigation</h2>
            <p>Choose a page to switch between user, dashboard, and operations views.</p>
          </div>

          <nav className="sidebar__nav" aria-label="Primary navigation">
            {navigationGroups.map((group) => (
              <div key={group.label} className="nav-group">
                <span className="nav-group__label">{group.label}</span>
                <div className="nav-group__items">
                  {group.items.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      className={`nav-item ${activeView === item.id ? "nav-item--active" : ""}`}
                      onClick={() => openView(item.id)}
                    >
                      <div className="nav-item__top">
                        <strong>{item.label}</strong>
                        <span>{activeView === item.id ? "Open" : "View"}</span>
                      </div>
                      <p>{item.description}</p>
                      <div className="nav-item__meta">
                        {item.meta.map((meta) => (
                          <span key={meta} className="meta-pill">
                            {meta}
                          </span>
                        ))}
                      </div>
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </nav>

          <div className="sidebar__panel sidebar__panel--compact">
            <span className="sidebar__eyebrow">Live snapshot</span>
            <div className="sidebar-stats">
              <div>
                <strong>{summary ? summary.total_gpus : "—"}</strong>
                <span>Total GPUs</span>
              </div>
              <div>
                <strong>{summary ? summary.available_gpus : "—"}</strong>
                <span>Available</span>
              </div>
              <div>
                <strong>{workloadSummary.pending}</strong>
                <span>Pending</span>
              </div>
              <div>
                <strong>{workloadSummary.running}</strong>
                <span>Running</span>
              </div>
            </div>
          </div>
        </aside>

        <div className="workspace__main" id="page-content">
          <section className="content-section">
            <div className="content-section__header">
              <div>
                <span className="content-section__eyebrow">{activePage.eyebrow}</span>
                <h2>{activePage.title}</h2>
                <p>{activePage.description}</p>
              </div>
              <div className="content-section__switcher" role="tablist" aria-label="View switcher">
                {Object.entries(viewMeta).map(([view, meta]) => (
                  <button
                    key={view}
                    type="button"
                    className={`switcher-pill ${activeView === view ? "switcher-pill--active" : ""}`}
                    onClick={() => openView(view as ViewKey)}
                  >
                    {meta.eyebrow}
                  </button>
                ))}
              </div>
            </div>

            {activeView === "user-view" && (
              <div className="dashboard-grid">
                <section className="panel panel--span-7">
                  <div className="panel__header">
                    <div>
                      <h3>Submit Workload</h3>
                      <p>Create a workload against the current fleet state.</p>
                    </div>
                  </div>
                  <form onSubmit={onSubmit} className="form-grid">
                  <label>
                    Type
                    <select name="type" value={workloadType} onChange={(event) => setWorkloadType(event.target.value as Workload["type"])}>
                      <option value="training">training</option>
                      <option value="inference">inference</option>
                      <option value="batch">batch</option>
                    </select>
                  </label>
                    <label>
                      GPU Type
                      <select name="gpu_type" defaultValue="A100">
                        <option value="H100">H100</option>
                        <option value="A100">A100</option>
                        <option value="L4">L4</option>
                      </select>
                    </label>
                    <label>
                      GPU Count
                      <input name="gpu_count" type="number" min={1} defaultValue={1} required />
                    </label>
                    <label>
                      Priority
                      <select name="priority" defaultValue="normal">
                        <option value="high">high</option>
                        <option value="normal">normal</option>
                        <option value="low">low</option>
                      </select>
                    </label>
                  <label>
                    Duration Seconds
                    <input name="duration_seconds" type="number" min={1} defaultValue={300} required />
                  </label>
                  {workloadType === "inference" ? (
                    <label>
                      Replicas
                      <input name="replicas" type="number" min={1} defaultValue={2} />
                    </label>
                  ) : null}
                  <label className="checkbox">
                    <input name="spot_tolerant" type="checkbox" defaultChecked />
                    Spot tolerant
                    </label>
                    <label className="checkbox">
                      <input name="resumable" type="checkbox" />
                      Resumable
                    </label>
                    <button className="button button--primary button--form" disabled={submitting} type="submit">
                      {submitting ? "Submitting..." : "Submit workload"}
                    </button>
                  </form>
                  {result && (
                    <div className="inline-card">
                      <div className="inline-card__title">
                        <span>Last submission</span>
                        <strong>{result.id}</strong>
                      </div>
                      <div className="inline-card__body">
                        <span className={`chip chip--${tone(result.state)}`}>{result.state}</span>
                        <span>{result.type}</span>
                      <span>
                        {result.gpu_type} x {result.gpu_count}
                      </span>
                      {result.type === "inference" && <span>{result.replicas ?? 1} replica(s)</span>}
                      <span>{result.priority}</span>
                      {result.resumable && <span>Resumable</span>}
                      {result.placement?.node_id && <span>Placed on {result.placement.node_id}</span>}
                      {result.replica_placements?.length ? (
                        <span>{result.replica_placements.length} placement(s)</span>
                      ) : null}
                    </div>
                      {result.status_reason && <p className="muted">{result.status_reason}</p>}
                      {result.scheduling_explanation && <p className="muted">{result.scheduling_explanation}</p>}
                      {(result.preempt_notice_seconds || result.checkpoint_state || result.resume_eligible) && (
                        <div className="event-meta">
                          {result.preempt_notice_seconds ? (
                            <span className="event-meta__item">notice: {result.preempt_notice_seconds}s</span>
                          ) : null}
                          {result.checkpoint_state ? (
                            <span className="event-meta__item">checkpoint: {result.checkpoint_state}</span>
                          ) : null}
                          {result.resume_eligible ? (
                            <span className="event-meta__item">resume eligible</span>
                          ) : null}
                        </div>
                      )}
                    </div>
                  )}
                </section>

                <section className="panel panel--span-5">
                  <div className="panel__header">
                    <div>
                      <h3>Workload Monitoring</h3>
                      <p>Placement, queueing, and explanation details from the live backend.</p>
                    </div>
                    <span className="panel__badge">{workloads.length} workloads</span>
                  </div>
                  <div className="table-wrap">
                    <table>
                      <thead>
                      <tr>
                        <th>ID</th>
                        <th>Type</th>
                        <th>GPU</th>
                        <th>Count</th>
                        <th>Replicas</th>
                        <th>Priority</th>
                        <th>State</th>
                        <th>Placement / Reason</th>
                      </tr>
                      </thead>
                      <tbody>
                        {workloads.map((workload) => (
                          <tr key={workload.id}>
                            <td className="mono">{workload.id}</td>
                            <td>{workload.type}</td>
                            <td>{workload.gpu_type}</td>
                          <td>{workload.gpu_count}</td>
                          <td>{workload.type === "inference" ? `${workload.replicas ?? 1}x` : "—"}</td>
                          <td>{workload.priority}</td>
                          <td>
                            <span className={`chip chip--${tone(workload.state)}`}>{workload.state}</span>
                          </td>
                          <td className="cell-stack">
                              {workload.placement?.node_id ? (
                                <span className="mono">{workload.placement.node_id}</span>
                              ) : (
                                <span className="muted">{workload.status_reason || "Queued"}</span>
                              )}
                            {workload.scheduling_explanation && (
                              <span className="muted">{workload.scheduling_explanation}</span>
                            )}
                            {workload.type === "inference" && (workload.replica_placements?.length ?? 0) > 1 && (
                              <span className="muted">
                                {workload.replica_placements?.length ?? workload.replicas ?? 1} replica placements
                              </span>
                            )}
                            {(workload.preempt_notice_seconds || workload.checkpoint_state || workload.resume_eligible) && (
                              <div className="event-meta">
                                  {workload.preempt_notice_seconds ? (
                                    <span className="event-meta__item">{workload.preempt_notice_seconds}s notice</span>
                                  ) : null}
                                  {workload.checkpoint_state ? (
                                    <span className="event-meta__item">{workload.checkpoint_state}</span>
                                  ) : null}
                                  {workload.resume_eligible ? (
                                    <span className="event-meta__item">resume eligible</span>
                                  ) : null}
                                </div>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </section>
              </div>
            )}

            {activeView === "admin-dashboard" && (
              <>
                <section className="stats-grid">
                  <article className="stat-card">
                    <span className="stat-card__label">Total GPUs</span>
                    <strong>{summary ? summary.total_gpus : "—"}</strong>
                    <span>Allocated {summary ? summary.allocated_gpus : "—"}</span>
                  </article>
                  <article className="stat-card">
                    <span className="stat-card__label">Available GPUs</span>
                    <strong>{summary ? summary.available_gpus : "—"}</strong>
                    <span>
                      {summary ? `${summary.utilization_percent.toFixed(1)}% utilization` : "Waiting for summary"}
                    </span>
                  </article>
                  <article className="stat-card">
                    <span className="stat-card__label">Pending</span>
                    <strong>{workloadSummary.pending}</strong>
                    <span>Queued work awaiting a fit</span>
                  </article>
                  <article className="stat-card">
                    <span className="stat-card__label">Running</span>
                    <strong>{workloadSummary.running}</strong>
                    <span>Workloads currently placed</span>
                  </article>
                </section>

                <div className="dashboard-grid">
                  <section className="panel panel--span-7">
                    <div className="panel__header">
                      <div>
                        <h3>Fleet Summary</h3>
                        <p>Capacity, utilization, and node health in one view.</p>
                      </div>
                    </div>
                    {summary ? (
                      <div className="fleet-summary">
                        <div className="fleet-summary__row">
                          <span>Utilization</span>
                          <strong>{summary.utilization_percent.toFixed(1)}%</strong>
                        </div>
                        <div className="progress">
                          <div
                            className="progress__bar"
                            style={{ width: `${Math.max(0, Math.min(100, summary.utilization_percent))}%` }}
                          />
                        </div>
                        <dl className="fleet-summary__grid">
                          <div>
                            <dt>Total</dt>
                            <dd>{summary.total_gpus}</dd>
                          </div>
                          <div>
                            <dt>Allocated</dt>
                            <dd>{summary.allocated_gpus}</dd>
                          </div>
                          <div>
                            <dt>Available</dt>
                            <dd>{summary.available_gpus}</dd>
                          </div>
                          <div>
                            <dt>Healthy nodes</dt>
                            <dd>{healthyNodes.length}</dd>
                          </div>
                        </dl>
                        {summary.gpu_types && (
                          <div className="mini-table">
                            <div className="mini-table__head">
                              <span>GPU</span>
                              <span>Total</span>
                              <span>Allocated</span>
                            </div>
                            {Object.entries(summary.gpu_types).map(([gpuType, values]) => (
                              <div key={gpuType} className="mini-table__row">
                                <span>{gpuType}</span>
                                <span>{values.total}</span>
                                <span>{values.allocated}</span>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ) : (
                      <p className="muted">Loading summary...</p>
                    )}
                    <div className="node-strip">
                      {nodes.map((node) => (
                        <button
                          key={node.id}
                          type="button"
                          className={`node-pill ${selectedNodeId === node.id ? "node-pill--active" : ""}`}
                          onClick={() => setSelectedNodeId(node.id)}
                        >
                          <strong>{node.id}</strong>
                          <span>{node.gpu_type}</span>
                          <span>{node.health}</span>
                        </button>
                      ))}
                    </div>
                  </section>

                  <section className="panel panel--span-5">
                    <div className="panel__header">
                      <div>
                        <h3>Event Log</h3>
                        <p>Recent scheduling and admin events from the API.</p>
                      </div>
                      <span className="panel__badge">{events.length} recent</span>
                    </div>

                    <div className="event-list">
                      {events.length === 0 ? (
                        <p className="muted">No events loaded yet.</p>
                      ) : (
                        events.map((event) => (
                          <article key={event.id} className="event-item">
                            <div className="event-item__top">
                              <span className={`chip chip--${eventTone(event.type)}`}>{event.type}</span>
                              <span className="mono">{formatTimestamp(event.timestamp)}</span>
                            </div>
                            <div className="event-item__body">
                              <strong>{event.message}</strong>
                              <span>
                                {event.actor}
                                {event.node_id ? ` · node ${event.node_id}` : ""}
                                {event.workload_id ? ` · workload ${event.workload_id}` : ""}
                              </span>
                              {event.metadata && Object.keys(event.metadata).length > 0 && (
                                <div className="event-meta">
                                  {Object.entries(event.metadata).map(([key, value]) => (
                                    <span key={key} className="event-meta__item">
                                      {key}: {value}
                                    </span>
                                  ))}
                                </div>
                              )}
                            </div>
                          </article>
                        ))
                      )}
                    </div>
                  </section>
                </div>
              </>
            )}

            {activeView === "admin-ops" && (
              <div className="dashboard-grid dashboard-grid--ops">
                <section className="panel panel--span-5">
                  <div className="panel__header">
                    <div>
                      <h3>Operational shortcuts</h3>
                      <p>Reset demo data or advance the scheduler without leaving the page.</p>
                    </div>
                  </div>

                  <div className="ops-stack">
                    <div className="ops-card">
                      <div className="ops-card__copy">
                        <strong>Seed demo data</strong>
                        <span>Restore the deterministic fleet, workload, and event set.</span>
                      </div>
                      <button
                        className="button button--secondary"
                        onClick={() => handleDemoAction("seed")}
                        disabled={demoAction !== null || loading}
                        type="button"
                      >
                        {demoAction === "seed" ? "Seeding..." : "Seed demo data"}
                      </button>
                    </div>

                    <div className="ops-card">
                      <div className="ops-card__copy">
                        <strong>Clear data</strong>
                        <span>Wipe the current demo state and start from a clean slate.</span>
                      </div>
                      <button
                        className="button button--secondary"
                        onClick={() => handleDemoAction("clear")}
                        disabled={demoAction !== null || loading}
                        type="button"
                      >
                        {demoAction === "clear" ? "Clearing..." : "Clear data"}
                      </button>
                    </div>

                    <div className="ops-card">
                      <div className="ops-card__copy">
                        <strong>Scheduler tick</strong>
                        <span>Advance pending placements and observe the scheduling decisions.</span>
                      </div>
                      <button className="button button--secondary" onClick={handleTick} disabled={tickLoading} type="button">
                        {tickLoading ? "Ticking..." : "Run scheduler tick"}
                      </button>
                    </div>
                  </div>

                  <p className="muted">
                    Use these controls to cycle the demo environment and observe how the scheduler responds.
                  </p>
                </section>

                <section className="panel panel--span-7">
                  <div className="panel__header">
                    <div>
                      <h3>Node disruption controls</h3>
                      <p>Choose a node, then apply failure, recovery, or spot preemption.</p>
                    </div>
                    <span className="panel__badge">{nodes.length} nodes</span>
                  </div>

                  <div className="control-stack">
                    <label>
                      Target node
                      <select value={selectedNodeId} onChange={(event) => setSelectedNodeId(event.target.value)}>
                        {nodes.length === 0 ? <option value="">No nodes loaded</option> : null}
                        {nodes.map((node) => (
                          <option key={node.id} value={node.id}>
                            {node.id} · {node.gpu_type} · {node.health}
                          </option>
                        ))}
                      </select>
                    </label>

                    {selectedNode ? (
                      <div className="node-details">
                        <div>
                          <strong>{selectedNode.id}</strong>
                          <span>
                            {selectedNode.provider} · {selectedNode.region} · {selectedNode.zone}
                          </span>
                        </div>
                        <div className="node-details__chips">
                          <span className={`chip chip--${tone(selectedNode.health)}`}>{selectedNode.health}</span>
                          <span className="chip chip--neutral">{selectedNode.capacity_class}</span>
                          <span className="chip chip--neutral">
                            {clampCount(selectedNode.allocated_gpus)}/{clampCount(selectedNode.total_gpus)} GPUs
                          </span>
                        </div>
                      </div>
                    ) : (
                      <p className="muted">Select a node to see its current state.</p>
                    )}

                    <div className="action-grid">
                      <button
                        className="button button--danger"
                        onClick={() => handleNodeAction("fail")}
                        disabled={!selectedNodeId || adminAction !== null || !canFailNode}
                        type="button"
                        title={selectedNode?.health === "failed" ? "This node is already failed." : undefined}
                      >
                        {adminAction === "fail" ? "Failing..." : "Fail node"}
                      </button>
                      <button
                        className="button button--secondary"
                        onClick={() => handleNodeAction("recover")}
                        disabled={!selectedNodeId || adminAction !== null || !canRecoverNode}
                        type="button"
                        title={
                          selectedNode && selectedNode.health !== "failed"
                            ? "Recover only applies to failed nodes."
                            : undefined
                        }
                      >
                        {adminAction === "recover" ? "Recovering..." : "Recover node"}
                      </button>
                      <button
                        className="button button--secondary"
                        onClick={() => handleNodeAction("preempt-spot")}
                        disabled={!selectedNodeId || adminAction !== null || !canPreemptNode}
                        type="button"
                        title={
                          selectedNode && selectedNode.capacity_class !== "spot"
                            ? "Preemption only applies to spot nodes."
                            : undefined
                        }
                      >
                        {adminAction === "preempt-spot" ? "Preempting..." : "Preempt spot"}
                      </button>
                    </div>

                    <p className="muted">
                      Failed and preempted workloads are reflected after the automatic refresh.
                    </p>
                  </div>
                </section>
              </div>
            )}
          </section>
        </div>
      </div>
    </main>
  );
}
