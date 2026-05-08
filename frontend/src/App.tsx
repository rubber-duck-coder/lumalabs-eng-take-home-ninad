import { CSSProperties, FormEvent, useEffect, useMemo, useState } from "react";

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

type TelemetrySnapshot = {
  timestamp: string;
  total_gpus: number;
  allocated_gpus: number;
  available_gpus: number;
  failed_gpus?: number;
  utilization_percent: number;
  healthy_nodes: number;
  recovering_nodes: number;
  failed_nodes: number;
  pending_workloads: number;
  running_workloads: number;
  completed_workloads?: number;
  suspended_workloads?: number;
};

type FleetSummary = {
  total_gpus: number;
  allocated_gpus: number;
  available_gpus: number;
  failed_gpus?: number;
  utilization_percent: number;
  gpu_types?: Record<string, { total: number; allocated: number; available?: number; failed?: number }>;
  workloads_by_state?: Record<string, number>;
};

type NodeAction = "fail" | "recover" | "preempt-spot";
type SimulationScenario = "sudden-inference-spike" | "spot-preemption" | "node-failures" | "capacity-exhausted";
type ViewKey = "user-view" | "admin-dashboard" | "admin-ops" | "system-design";
type UserTabKey = "submit" | "monitoring";
type DashboardTabKey = "summary" | "events";
type DesignTabKey = "high-level" | "api-sequences" | "limitations" | "decisions";
type DesignFlowKey = "submit" | "tick" | "disruption" | "simulation" | "read";

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

type SimulationResult = {
  scenario: SimulationScenario;
  message: string;
  workloads?: Workload[];
  disruptions?: DisruptionResult[];
  scheduled?: unknown[];
};

type TelemetrySeries = {
  label: string;
  color: string;
  values: number[];
};

const apiBase = import.meta.env.VITE_API_BASE_URL || "/api";

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
  const message = reason instanceof Error ? reason.message : String(reason);
  const lower = message.toLowerCase();
  if (lower.includes("invalid_")) return "Check the form fields and try again.";
  if (lower.includes("invalid disruption response")) return "The server returned an unexpected disruption response.";
  if (lower.includes("request failed")) return "The backend request failed.";
  if (lower.includes("not found")) return "The selected item is no longer available.";
  return message;
}

function shortText(value: string, maxLength = 120) {
  const normalized = value.trim().replace(/\s+/g, " ");
  if (normalized.length <= maxLength) return normalized;
  return `${normalized.slice(0, maxLength - 1).trimEnd()}…`;
}

function summarizeWorkloadReason(workload: Workload) {
  if (workload.type === "inference" && (workload.replica_placements?.length ?? 0) > 1) {
    return `${workload.replica_placements?.length ?? workload.replicas ?? 1} replicas placed`;
  }
  if (workload.placement?.node_id) {
    return `Placed on ${workload.placement.node_id}`;
  }
  if (workload.status_reason) {
    if (workload.state === "pending") {
      return `Queued: ${shortText(workload.status_reason, 96)}`;
    }
    return shortText(workload.status_reason, 96);
  }
  if (workload.scheduling_explanation) {
    return shortText(workload.scheduling_explanation, 96);
  }
  return "Queued";
}

function formatTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function titleize(value: string) {
  return value
    .replace(/[-_]/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatEventType(type: string) {
  return titleize(type);
}

function formatEventMessage(event: Event) {
  const scenario = event.metadata?.scenario;
  if (scenario) {
    if (event.type === "simulation_started") return `${titleize(scenario)} started`;
    if (event.type === "simulation_completed") return `${titleize(scenario)} completed`;
    if (event.type === "simulation_failed") return `${titleize(scenario)} failed`;
  }
  return shortText(event.message, 88);
}

function metadataEntries(event: Event) {
  const entries: Array<[string, string]> = [];
  if (event.node_id) entries.push(["target_node", event.node_id]);
  if (event.workload_id) entries.push(["target_workload", event.workload_id]);
  if (event.metadata) entries.push(...Object.entries(event.metadata));
  return entries;
}

function buildTelemetrySnapshot(
  summary: FleetSummary | null,
  nodes: Node[],
  workloads: Workload[]
): TelemetrySnapshot {
  const totalGPUs =
    summary?.total_gpus ?? nodes.reduce((total, node) => total + node.total_gpus, 0);
  const allocatedGPUs =
    summary?.allocated_gpus ?? nodes.reduce((total, node) => total + node.allocated_gpus, 0);
  const availableGPUs = summary?.available_gpus ?? Math.max(0, totalGPUs - allocatedGPUs);
  const failedGPUs =
    summary?.failed_gpus ?? nodes.filter((node) => node.health === "failed").reduce((total, node) => total + node.total_gpus, 0);
  const utilizationPercent =
    summary?.utilization_percent ?? (totalGPUs > 0 ? (allocatedGPUs / totalGPUs) * 100 : 0);

  return {
    timestamp: new Date().toISOString(),
    total_gpus: totalGPUs,
    allocated_gpus: allocatedGPUs,
    available_gpus: availableGPUs,
    failed_gpus: failedGPUs,
    utilization_percent: utilizationPercent,
    healthy_nodes: nodes.filter((node) => node.health === "healthy").length,
    recovering_nodes: nodes.filter((node) => node.health === "recovering").length,
    failed_nodes: nodes.filter((node) => node.health === "failed").length,
    pending_workloads: workloads.filter((workload) => workload.state === "pending").length,
    running_workloads: workloads.filter((workload) => workload.state === "running").length,
    completed_workloads: workloads.filter((workload) => workload.state === "completed").length,
    suspended_workloads: workloads.filter((workload) => workload.state === "preempted").length
  };
}

function mergeTelemetrySnapshots(existing: TelemetrySnapshot[], incoming: TelemetrySnapshot[]) {
  const merged = new Map<string, TelemetrySnapshot>();
  for (const snapshot of existing) {
    merged.set(snapshot.timestamp, snapshot);
  }
  for (const snapshot of incoming) {
    merged.set(snapshot.timestamp, snapshot);
  }
  return Array.from(merged.values()).sort(
    (left, right) => new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime()
  );
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

function lastValue(values: number[]) {
  return values.length > 0 ? values[values.length - 1] : 0;
}

function formatChartTime(value?: string) {
  if (!value) return "time";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "time" : date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function chartMinMax(series: TelemetrySeries[]) {
  const values = series.flatMap((entry) => entry.values);
  const max = values.length > 0 ? Math.max(...values) : 1;
  return { min: 0, max: Math.max(1, max) };
}

function chartPoints(values: number[], width: number, height: number, min: number, max: number) {
  if (values.length === 0) {
    return "";
  }

  const usableWidth = width - 16;
  const usableHeight = height - 16;
  const step = values.length === 1 ? 0 : usableWidth / (values.length - 1);
  const range = max - min || 1;

  return values
    .map((value, index) => {
      const x = 8 + step * index;
      const y = 8 + usableHeight - ((value - min) / range) * usableHeight;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
}

function TelemetryChart({
  title,
  subtitle,
  series,
  timestamps,
  yLabel = "count",
}: {
  title: string;
  subtitle: string;
  series: TelemetrySeries[];
  timestamps: string[];
  yLabel?: string;
}) {
  const width = 320;
  const height = 128;
  const { min, max } = chartMinMax(series);

  return (
    <article className="telemetry-chart">
      <div className="telemetry-chart__header">
        <div>
          <h4>{title}</h4>
          <span>{subtitle}</span>
        </div>
        <div className="telemetry-chart__badges">
          {series.map((entry) => (
            <span key={entry.label} className="meta-pill telemetry-chart__badge">
              <span className="telemetry-chart__swatch" style={{ backgroundColor: entry.color }} />
              {entry.label}: {lastValue(entry.values)}
            </span>
          ))}
        </div>
      </div>
      <svg className="telemetry-chart__svg" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={title}>
        <line x1="8" y1={height - 8} x2={width - 8} y2={height - 8} className="telemetry-chart__axis" />
        <line x1="8" y1="8" x2="8" y2={height - 8} className="telemetry-chart__axis" />
        {series.map((entry) => (
          <polyline
            key={entry.label}
            className="telemetry-chart__line"
            points={chartPoints(entry.values, width, height, min, max)}
            stroke={entry.color}
          />
        ))}
      </svg>
      <div className="telemetry-chart__axis-labels">
        <span>Y: {yLabel}</span>
        <span>
          X: time {formatChartTime(timestamps[0])} - {formatChartTime(timestamps[timestamps.length - 1])}
        </span>
      </div>
    </article>
  );
}

export function App() {
  const [workloads, setWorkloads] = useState<Workload[]>([]);
  const [summary, setSummary] = useState<FleetSummary | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [telemetry, setTelemetry] = useState<TelemetrySnapshot[]>([]);
  const [result, setResult] = useState<Workload | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");
  const [workloadType, setWorkloadType] = useState<Workload["type"]>("training");
  const [error, setError] = useState<string>("");
  const [statusMessage, setStatusMessage] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [adminAction, setAdminAction] = useState<NodeAction | null>(null);
  const [demoAction, setDemoAction] = useState<"seed" | "clear" | null>(null);
  const [simulationAction, setSimulationAction] = useState<SimulationScenario | null>(null);
  const [tickLoading, setTickLoading] = useState(false);
  const [activeView, setActiveView] = useState<ViewKey>("user-view");
  const [userTab, setUserTab] = useState<UserTabKey>("submit");
  const [dashboardTab, setDashboardTab] = useState<DashboardTabKey>("summary");
  const [designTab, setDesignTab] = useState<DesignTabKey>("high-level");
  const [designFlow, setDesignFlow] = useState<DesignFlowKey>("submit");
  const telemetryUtilizationSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.utilization_percent),
    [telemetry]
  );
  const telemetryAvailableSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.available_gpus),
    [telemetry]
  );
  const telemetryAllocatedSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.allocated_gpus),
    [telemetry]
  );
  const telemetryFailedGPUSeries = useMemo(() => telemetry.map((snapshot) => snapshot.failed_gpus ?? 0), [telemetry]);
  const telemetryHealthySeries = useMemo(() => telemetry.map((snapshot) => snapshot.healthy_nodes), [telemetry]);
  const telemetryRecoveringSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.recovering_nodes),
    [telemetry]
  );
  const telemetryFailedSeries = useMemo(() => telemetry.map((snapshot) => snapshot.failed_nodes), [telemetry]);
  const telemetryRunningWorkloadSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.running_workloads),
    [telemetry]
  );
  const telemetryPendingWorkloadSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.pending_workloads),
    [telemetry]
  );
  const telemetryCompletedWorkloadSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.completed_workloads ?? 0),
    [telemetry]
  );
  const telemetrySuspendedWorkloadSeries = useMemo(
    () => telemetry.map((snapshot) => snapshot.suspended_workloads ?? 0),
    [telemetry]
  );
  const latestTelemetry = telemetry.length > 0 ? telemetry[telemetry.length - 1] : null;
  const telemetryTimestamps = useMemo(() => telemetry.map((snapshot) => snapshot.timestamp), [telemetry]);

  async function refreshAll() {
    setLoading(true);
    const [workloadsResult, summaryResult, nodesResult, eventsResult, telemetryResult] = await Promise.allSettled([
      requestJSON<Workload[]>("/workloads"),
      requestJSON<FleetSummary>("/fleet/summary"),
      requestJSON<Node[]>("/nodes"),
      requestJSON<Event[]>("/events"),
      requestJSON<TelemetrySnapshot[]>("/telemetry?limit=180")
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

    if (telemetryResult.status === "fulfilled") {
      if (telemetryResult.value.length > 0) {
        setTelemetry((current) => mergeTelemetrySnapshots(current, telemetryResult.value).slice(-180));
      } else {
        const liveSnapshot = buildTelemetrySnapshot(
          summaryResult.status === "fulfilled" ? summaryResult.value : null,
          nodesResult.status === "fulfilled" ? nodesResult.value : [],
          workloadsResult.status === "fulfilled" ? workloadsResult.value : []
        );
        setTelemetry((current) => [...current.slice(-179), liveSnapshot]);
      }
    } else {
      const liveSnapshot = buildTelemetrySnapshot(
        summaryResult.status === "fulfilled" ? summaryResult.value : null,
        nodesResult.status === "fulfilled" ? nodesResult.value : [],
        workloadsResult.status === "fulfilled" ? workloadsResult.value : []
      );
      setTelemetry((current) => [...current.slice(-179), liveSnapshot]);
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
      if (hash === "user-view" || hash === "admin-dashboard" || hash === "admin-ops" || hash === "system-design") {
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
          meta: [`${workloadSummary.total} workloads`, `${workloadSummary.active} active`]
        },
        {
          id: "admin-dashboard" as const,
          label: "Admin dashboard",
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
          meta: [`${failedNodes.length} failed`, `${recoveringNodes.length} recovering`]
        }
      ]
    },
    {
      label: "Architecture",
      items: [
        {
          id: "system-design" as const,
          label: "System design overview",
          meta: ["Request flow", "Scheduling + reconcile"]
        }
      ]
    }
  ];

  const viewMeta: Record<ViewKey, { eyebrow: string; title: string }> = {
    "user-view": {
      eyebrow: "User",
      title: "Submit workloads and monitor placement"
    },
    "admin-dashboard": {
      eyebrow: "Dashboard",
      title: "System health, metrics, and event history"
    },
    "admin-ops": {
      eyebrow: "Ops",
      title: "Node disruption controls"
    },
    "system-design": {
      eyebrow: "Architecture",
      title: "Control plane system design overview"
    }
  };

  function openView(view: ViewKey) {
    setActiveView(view);
    window.history.replaceState(null, "", `#${view}`);
    document.getElementById("page-content")?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    setError("");
    setStatusMessage("");
    setSubmitting(true);
    const data = new FormData(form);
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
      form.reset();
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

  async function handleSimulation(scenario: SimulationScenario) {
    setError("");
    setStatusMessage("");
    setSimulationAction(scenario);

    try {
      const response = await requestJSON<SimulationResult>(`/admin/simulations/${scenario}`, {
        method: "POST"
      });
      setStatusMessage(response.message);
      await refreshAll();
    } catch (err) {
      setError(formatError(err));
    } finally {
      setSimulationAction(null);
    }
  }

  const activePage = viewMeta[activeView];
  const simulationOptions: Array<{ id: SimulationScenario; label: string; signal: string }> = [
    { id: "sudden-inference-spike", label: "Sudden inference spike", signal: "inference demand" },
    { id: "spot-preemption", label: "Spot preemption", signal: "spot interruption" },
    { id: "node-failures", label: "Node failures", signal: "health degradation" },
    { id: "capacity-exhausted", label: "Capacity exhausted", signal: "pending queue" }
  ];
  const userTabs: Array<{ id: UserTabKey; label: string }> = [
    { id: "submit", label: "Submit workload" },
    { id: "monitoring", label: "Workload monitoring" }
  ];
  const dashboardTabs: Array<{ id: DashboardTabKey; label: string }> = [
    { id: "summary", label: "Fleet summary" },
    { id: "events", label: "Event log" }
  ];
  const designTabs: Array<{ id: DesignTabKey; label: string }> = [
    { id: "high-level", label: "High-level design" },
    { id: "api-sequences", label: "API sequences" },
    { id: "limitations", label: "Limitations" },
    { id: "decisions", label: "Decision logic" }
  ];
  const designFlows: Array<{ id: DesignFlowKey; label: string; api: string; tone: "success" | "warning" | "danger" | "neutral" }> = [
    { id: "submit", label: "Workload submit", api: "POST /workloads", tone: "success" },
    { id: "tick", label: "Scheduler tick", api: "POST /scheduler/tick", tone: "warning" },
    { id: "disruption", label: "Node disruption", api: "POST /admin/nodes/:id/{fail|recover|preempt-spot}", tone: "danger" },
    { id: "simulation", label: "Scenario simulation", api: "POST /admin/simulations/:scenario", tone: "neutral" },
    { id: "read", label: "Read models", api: "GET /fleet/summary | /events | /telemetry", tone: "neutral" }
  ];
  const selectedDesignFlow = designFlows.find((flow) => flow.id === designFlow) ?? designFlows[0];
  const sequenceParticipants = ["Client/Admin", "Gateway", "Control Plane", "Policy Engine", "Store (Postgres)"];
  const sequenceByFlow: Record<DesignFlowKey, Array<{ from: string; to: string; action: string }>> = {
    submit: [
      { from: "Client/Admin", to: "Gateway", action: "POST /workloads" },
      { from: "Gateway", to: "Control Plane", action: "validate + normalize request" },
      { from: "Control Plane", to: "Policy Engine", action: "placement decision + preemption check" },
      { from: "Policy Engine", to: "Store (Postgres)", action: "atomic write: workload + allocation" },
      { from: "Control Plane", to: "Store (Postgres)", action: "emit event + telemetry snapshot" }
    ],
    tick: [
      { from: "Client/Admin", to: "Gateway", action: "POST /scheduler/tick" },
      { from: "Gateway", to: "Control Plane", action: "load pending queue" },
      { from: "Control Plane", to: "Policy Engine", action: "priority + class-aware ordering" },
      { from: "Policy Engine", to: "Store (Postgres)", action: "atomic placements for runnable workloads" },
      { from: "Control Plane", to: "Store (Postgres)", action: "persist scheduling events + telemetry" }
    ],
    disruption: [
      { from: "Client/Admin", to: "Gateway", action: "POST /admin/nodes/:id/*" },
      { from: "Gateway", to: "Control Plane", action: "validate disruption intent" },
      { from: "Control Plane", to: "Policy Engine", action: "evict/reschedule/queue policy" },
      { from: "Policy Engine", to: "Store (Postgres)", action: "atomic node + workload state transitions" },
      { from: "Control Plane", to: "Store (Postgres)", action: "persist disruption events + telemetry" }
    ],
    simulation: [
      { from: "Client/Admin", to: "Gateway", action: "POST /admin/simulations/:scenario" },
      { from: "Gateway", to: "Control Plane", action: "trigger composed control-plane actions" },
      { from: "Control Plane", to: "Policy Engine", action: "apply scenario-specific scheduling/disruption policy" },
      { from: "Policy Engine", to: "Store (Postgres)", action: "persist scenario state transitions" },
      { from: "Control Plane", to: "Store (Postgres)", action: "persist simulation_started/completed + telemetry trail" }
    ],
    read: [
      { from: "Client/Admin", to: "Gateway", action: "GET summary/events/telemetry" },
      { from: "Gateway", to: "Store (Postgres)", action: "query read models" },
      { from: "Store (Postgres)", to: "Gateway", action: "return summary + event log + timeseries" },
      { from: "Gateway", to: "Client/Admin", action: "render operational state" }
    ]
  };
  const limitationSections: Array<{
    id: string;
    title: string;
    note: string;
    refs?: string;
    future: string;
  }> = [
    {
      id: "scalability",
      title: "Scalability bottlenecks",
      note:
        "State writes currently favor snapshot-style full rewrites in Postgres paths and unpaginated list reads, which can amplify write/read cost as node/workload/event volume grows.",
      refs: "internal/store/postgres.go, internal/gateway/router.go",
      future:
        "Move to row-level CRUD + append-only event writes, and introduce pagination/filtering with bounded event windows."
    },
    {
      id: "availability",
      title: "Availability and multi-region resilience",
      note:
        "Control plane and persistence remain single-failure-domain oriented; multi-replica control plane would currently duplicate background loops without leader election.",
      refs: "cmd/control-plane/main.go, internal/store/config.go",
      future:
        "Adopt leader-elected workers plus multi-AZ/multi-region durable datastore and explicit failover semantics."
    },
    {
      id: "reconcile-overhead",
      title: "Scheduling and reconciliation overhead",
      note:
        "Reconcile and tick flows do repeated broad scans and coupled policy+mutation paths, which raises overhead and tuning complexity under high churn.",
      refs: "internal/controlplane/service.go, internal/store/memory.go",
      future:
        "Shift to event-driven incremental reconcile and keep scheduling policy output declarative before mutation application."
    },
    {
      id: "consistency",
      title: "Consistency and integrity risks",
      note:
        "Concurrent writers can still race without entity-level optimistic versions, and event recording is best-effort in places, weakening strict audit guarantees.",
      refs: "internal/store/postgres.go, internal/events/recorder.go",
      future:
        "Add CAS/versioned writes, idempotency keys, and an outbox pattern to atomically couple state transition + audit/event emission."
    },
    {
      id: "observability",
      title: "Observability and explainability gaps",
      note:
        "Telemetry/event visibility is useful but not yet rich enough for deep root-cause analysis at scale (for example structured decision trace and correlation IDs).",
      refs: "internal/telemetry/telemetry.go, internal/domain/event.go",
      future:
        "Add structured reason codes, correlation IDs, policy-score traces, and OpenTelemetry spans/metrics for schedule and reconcile paths."
    },
    {
      id: "security",
      title: "Security and tenancy",
      note:
        "Mutating APIs are currently open within network reach and designed for demo use, without full authn/authz, tenant boundaries, or admin/user separation.",
      refs: "internal/gateway/router.go",
      future:
        "Introduce identity-aware gateway middleware, RBAC, tenant-scoped resources, and policy/rate limits per actor and endpoint."
    },
    {
      id: "ops",
      title: "Operational risk",
      note:
        "Operational safety still depends on environment discipline; demo endpoints and in-memory fallback paths can become footguns in misconfigured environments.",
      refs: "internal/store/config.go, internal/gateway/router.go",
      future:
        "Fail closed for non-dev misconfigurations, hard-gate demo endpoints, and add safer automated deploy/runbook controls."
    },
    {
      id: "testing",
      title: "Testing and failure-mode coverage",
      note:
        "Coverage is solid for functional behavior but still light on high-risk production modes: Postgres concurrency/fault injection, graceful shutdown, and large-scale load/chaos.",
      refs: "internal/store/postgres_test.go, cmd/control-plane/main.go",
      future:
        "Add load/chaos suites with SLO assertions, multi-node fault injection, and deterministic replay tests for scheduler/reconciler decisions."
    }
  ];
  const decisionSections: Array<{
    id: string;
    title: string;
    detail: string;
    implementation: string;
  }> = [
    {
      id: "fit-first",
      title: "1) Feasibility first: hard-constraint filtering",
      detail:
        "Every placement starts with hard filters: GPU type and count, node health, and capacity-class compatibility. Nodes that violate any constraint are rejected before scoring.",
      implementation:
        "This prevents invalid placements and keeps explanations actionable by preserving explicit rejection reasons per candidate node."
    },
    {
      id: "class-aware",
      title: "2) Workload-class aware scheduling strategy",
      detail:
        "Training prefers stable on-demand capacity, inference prefers low-contention placement and spread, and batch prefers lower-cost spot when tolerated.",
      implementation:
        "Class-aware ordering and scoring keep latency-sensitive work responsive while preserving throughput/cost efficiency for flexible jobs."
    },
    {
      id: "binpack-vs-spread",
      title: "3) Bin packing vs spread: policy tradeoff",
      detail:
        "The system uses mixed strategy: tighter packing for training/batch to reduce fragmentation, and more spread for inference replicas to reduce correlated failure risk.",
      implementation:
        "Inference replica placement is node-distinct where possible; non-inference placement stays closer to bin-pack behavior under constraints."
    },
    {
      id: "preemption",
      title: "4) Preemption decision policy",
      detail:
        "Preemption is used when higher-priority work cannot be placed directly and lower-priority running workloads can be reclaimed to create a valid fit.",
      implementation:
        "Only policy-compatible victims are considered; affected workloads transition with explicit reasons/events, and resumable metadata is used when present."
    },
    {
      id: "queue-order",
      title: "5) Queue ordering and fairness",
      detail:
        "Pending workloads are revisited in deterministic policy order: priority first, then class-aware ordering within tier, with safeguards against uncontrolled churn.",
      implementation:
        "Deterministic ordering improves debuggability and repeatability for simulations and operator analysis."
    },
    {
      id: "reconcile",
      title: "6) Reconciliation loop behavior",
      detail:
        "Reconciliation reacts to fleet health shifts and pending backlog by re-running scheduling opportunities and recovery paths.",
      implementation:
        "The current approach emphasizes correctness and explainability over raw scale; event-driven/incremental reconcile is the next scaling step."
    },
    {
      id: "explainability",
      title: "7) Why a workload is queued or placed",
      detail:
        "For each decision, the system surfaces placement or queue explanations based on failed constraints and selected-node rationale.",
      implementation:
        "This is reflected in workload status reasons and event log metadata so admins can understand what happened, how, and why."
    },
    {
      id: "safety",
      title: "8) Safety over aggressiveness",
      detail:
        "The policy intentionally avoids hyper-aggressive rebalance to reduce unnecessary workload movement and operational instability.",
      implementation:
        "Anti-churn bias keeps behavior stable unless policy value is clear (priority pressure, disruption recovery, or explicit scheduling trigger)."
    }
  ];
  const supportedAPIs = [
    "GET /health",
    "GET /nodes",
    "GET /fleet/summary",
    "GET /events",
    "GET /telemetry?limit=n",
    "GET /workloads",
    "POST /workloads",
    "POST /scheduler/tick",
    "POST /admin/nodes/:id/fail",
    "POST /admin/nodes/:id/recover",
    "POST /admin/nodes/:id/preempt-spot",
    "POST /admin/demo/seed",
    "POST /admin/demo/clear",
    "POST /admin/simulations/:scenario"
  ];

  return (
    <main className="page">
      {statusMessage && <div className="banner banner--success">{statusMessage}</div>}
      {error && <div className="banner banner--error">{error}</div>}

      <div className="workspace">
        <aside className="sidebar" aria-label="Dashboard navigation and overview">
          <div className="sidebar__panel">
            <span className="sidebar__eyebrow">Overview</span>
            <h2>Fleet console</h2>
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

        </aside>

        <div className="workspace__main" id="page-content">
          <section className="content-section">
            <div className="content-section__header">
              <div>
                <span className="content-section__eyebrow">{activePage.eyebrow}</span>
                <h2>{activePage.title}</h2>
              </div>
            </div>

            {activeView === "user-view" && (
              <div className="tab-view">
                <div className="tab-bar" role="tablist" aria-label="User view tabs">
                  {userTabs.map((tab) => (
                    <button
                      key={tab.id}
                      type="button"
                      className={`tab-pill ${userTab === tab.id ? "tab-pill--active" : ""}`}
                      onClick={() => setUserTab(tab.id)}
                    >
                      {tab.label}
                    </button>
                  ))}
                </div>

                {userTab === "submit" && (
                  <section className="panel">
                    <div className="panel__header">
                      <h3>Submit Workload</h3>
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
                )}

                {userTab === "monitoring" && (
                  <section className="panel">
                    <div className="panel__header">
                      <h3>Workload Monitoring</h3>
                      <span className="panel__badge">{workloads.length} workloads</span>
                    </div>
                    <div className="table-wrap">
                      <table className="data-table">
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
                              <td className="cell-stack cell-stack--compact" title={summarizeWorkloadReason(workload)}>
                                <span className="mono">{summarizeWorkloadReason(workload)}</span>
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
                )}
              </div>
            )}

            {activeView === "admin-dashboard" && (
              <div className="tab-view">
                <div className="tab-bar" role="tablist" aria-label="Admin dashboard tabs">
                  {dashboardTabs.map((tab) => (
                    <button
                      key={tab.id}
                      type="button"
                      className={`tab-pill ${dashboardTab === tab.id ? "tab-pill--active" : ""}`}
                      onClick={() => setDashboardTab(tab.id)}
                    >
                      {tab.label}
                    </button>
                  ))}
                  <div className="dashboard-actions">
                    <button
                      className="button button--secondary button--tiny"
                      onClick={() => refreshAll().catch((err) => setError(formatError(err)))}
                      disabled={loading}
                      type="button"
                    >
                      {loading ? "Refreshing" : "Refresh"}
                    </button>
                    <button
                      className="button button--secondary button--tiny"
                      onClick={handleTick}
                      disabled={tickLoading}
                      type="button"
                    >
                      {tickLoading ? "Ticking" : "Scheduler tick"}
                    </button>
                  </div>
                </div>

                {dashboardTab === "summary" && (
                  <>
                    <section className="simulation-panel" aria-label="Simulation scenarios">
                      <div className="simulation-panel__header">
                        <h3>Run simulations</h3>
                        <span>Trigger a scenario and observe telemetry, fleet state, and events.</span>
                      </div>
                      <div className="simulation-strip">
                        {simulationOptions.map((scenario) => (
                          <button
                            key={scenario.id}
                            type="button"
                            className="simulation-pill"
                            onClick={() => handleSimulation(scenario.id)}
                            disabled={simulationAction !== null}
                          >
                            <span>{scenario.signal}</span>
                            <strong>{simulationAction === scenario.id ? "Running..." : scenario.label}</strong>
                          </button>
                        ))}
                      </div>
                    </section>

                    <section className="panel">
                      <div className="panel__header">
                        <h3>Telemetry</h3>
                        <span className="panel__badge">{telemetry.length} points</span>
                      </div>
                      <div className="telemetry-grid">
                        <TelemetryChart
                          title="Utilization"
                          subtitle={latestTelemetry ? `${latestTelemetry.utilization_percent.toFixed(1)}%` : "—"}
                          series={[{ label: "utilization", color: "#1d4ed8", values: telemetryUtilizationSeries }]}
                          timestamps={telemetryTimestamps}
                          yLabel="percent"
                        />
                        <TelemetryChart
                          title="GPU capacity"
                          subtitle={latestTelemetry ? `${latestTelemetry.available_gpus} schedulable` : "—"}
                          series={[
                            { label: "available", color: "#15803d", values: telemetryAvailableSeries },
                            { label: "allocated", color: "#b45309", values: telemetryAllocatedSeries },
                            { label: "failed", color: "#b91c1c", values: telemetryFailedGPUSeries }
                          ]}
                          timestamps={telemetryTimestamps}
                          yLabel="GPUs"
                        />
                        <TelemetryChart
                          title="Node health"
                          subtitle={latestTelemetry ? `${latestTelemetry.healthy_nodes} healthy` : "—"}
                          series={[
                            { label: "healthy", color: "#15803d", values: telemetryHealthySeries },
                            { label: "recovering", color: "#b45309", values: telemetryRecoveringSeries },
                            { label: "failed", color: "#b91c1c", values: telemetryFailedSeries }
                          ]}
                          timestamps={telemetryTimestamps}
                          yLabel="nodes"
                        />
                        <TelemetryChart
                          title="Workloads"
                          subtitle={
                            latestTelemetry
                              ? `${latestTelemetry.running_workloads} running, ${latestTelemetry.pending_workloads} queued`
                              : "—"
                          }
                          series={[
                            { label: "running", color: "#15803d", values: telemetryRunningWorkloadSeries },
                            { label: "queued", color: "#b45309", values: telemetryPendingWorkloadSeries },
                            { label: "completed", color: "#1d4ed8", values: telemetryCompletedWorkloadSeries },
                            { label: "suspended", color: "#b91c1c", values: telemetrySuspendedWorkloadSeries }
                          ]}
                          timestamps={telemetryTimestamps}
                          yLabel="workloads"
                        />
                      </div>
                    </section>

                    <section className="metric-strip">
                      <div>
                        <span>Total GPUs</span>
                        <strong>{summary ? summary.total_gpus : "—"}</strong>
                      </div>
                      <div>
                        <span>Allocated</span>
                        <strong>{summary ? summary.allocated_gpus : "—"}</strong>
                      </div>
                      <div>
                        <span>Available</span>
                        <strong>{summary ? summary.available_gpus : "—"}</strong>
                      </div>
                      <div>
                        <span>Failed</span>
                        <strong>{summary ? summary.failed_gpus ?? 0 : "—"}</strong>
                      </div>
                      <div>
                        <span>Pending</span>
                        <strong>{workloadSummary.pending}</strong>
                      </div>
                      <div>
                        <span>Running</span>
                        <strong>{workloadSummary.running}</strong>
                      </div>
                    </section>

                    <section className="panel">
                      <div className="panel__header">
                        <h3>Fleet Summary</h3>
                      </div>
                      {summary ? (
                        <div className="fleet-summary">
                          <div className="node-metric-strip">
                            <div>
                              <span>Total nodes</span>
                              <strong>{nodes.length}</strong>
                            </div>
                            <div className="node-metric node-metric--success">
                              <span>Healthy</span>
                              <strong>{healthyNodes.length}</strong>
                            </div>
                            <div className="node-metric node-metric--warning">
                              <span>Recovering</span>
                              <strong>{recoveringNodes.length}</strong>
                            </div>
                            <div className="node-metric node-metric--danger">
                              <span>Failed</span>
                              <strong>{failedNodes.length}</strong>
                            </div>
                          </div>
                          <div className="utilization-gauge">
                            <div
                              className="utilization-gauge__dial"
                              style={
                                {
                                  "--gauge-value": `${Math.max(0, Math.min(100, summary.utilization_percent)) * 1.8}deg`
                                } as CSSProperties
                              }
                            >
                              <strong>{summary.utilization_percent.toFixed(1)}%</strong>
                              <span>Utilization</span>
                            </div>
                          </div>
                          {summary.gpu_types && (
                            <div className="mini-table">
                              <div className="mini-table__head">
                                <span>GPU</span>
                                <span>Total</span>
                                <span>Allocated</span>
                                <span>Schedulable</span>
                                <span>Failed</span>
                              </div>
                              {Object.entries(summary.gpu_types).map(([gpuType, values]) => (
                                <div key={gpuType} className="mini-table__row">
                                  <span>{gpuType}</span>
                                  <span>{values.total}</span>
                                  <span>{values.allocated}</span>
                                  <span>{values.available ?? 0}</span>
                                  <span>{values.failed ?? 0}</span>
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
                  </>
                )}

                {dashboardTab === "events" && (
                  <section className="panel">
                    <div className="panel__header">
                      <h3>Event Log</h3>
                      <span className="panel__badge">{events.length} recent</span>
                    </div>

                    <div className="table-wrap">
                      <table className="data-table event-table">
                        <thead>
                          <tr>
                            <th>Time</th>
                            <th>Type</th>
                            <th>Message</th>
                            <th>Actor</th>
                            <th>Details</th>
                          </tr>
                        </thead>
                        <tbody>
                          {events.length === 0 ? (
                            <tr>
                              <td colSpan={5} className="table-empty">
                                No events loaded yet.
                              </td>
                            </tr>
                          ) : (
                            events.map((event) => (
                              <tr key={event.id}>
                                <td className="mono">{formatTimestamp(event.timestamp)}</td>
                                <td>
                                  <span className={`chip chip--${eventTone(event.type)}`}>{formatEventType(event.type)}</span>
                                </td>
                                <td className="cell-stack cell-stack--compact" title={event.message}>
                                  <span>{formatEventMessage(event)}</span>
                                </td>
                                <td className="mono">{event.actor}</td>
                                <td>
                                  {metadataEntries(event).length > 0 ? (
                                    <details className="event-details">
                                      <summary className="event-details__summary">
                                        <span>Details</span>
                                        <span className="event-details__chevron">▾</span>
                                      </summary>
                                      <div className="event-meta">
                                        {metadataEntries(event).map(([key, value]) => (
                                          <span key={key} className="event-meta__item">
                                            {titleize(key)}: {value}
                                          </span>
                                        ))}
                                      </div>
                                    </details>
                                  ) : (
                                    "—"
                                  )}
                                </td>
                              </tr>
                            ))
                          )}
                        </tbody>
                      </table>
                    </div>
                  </section>
                )}
              </div>
            )}

            {activeView === "admin-ops" && (
              <div className="dashboard-grid dashboard-grid--ops">
                <section className="panel panel--span-5">
                  <div className="panel__header">
                    <h3>Operational shortcuts</h3>
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

                </section>

                <section className="panel panel--span-7">
                  <div className="panel__header">
                    <h3>Node disruption controls</h3>
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
                    ) : null}

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

                  </div>
                </section>
              </div>
            )}

            {activeView === "system-design" && (
              <div className="tab-view">
                <div className="tab-bar" role="tablist" aria-label="System design tabs">
                  {designTabs.map((tab) => (
                    <button
                      key={tab.id}
                      type="button"
                      className={`tab-pill ${designTab === tab.id ? "tab-pill--active" : ""}`}
                      onClick={() => setDesignTab(tab.id)}
                    >
                      {tab.label}
                    </button>
                  ))}
                </div>

                {designTab === "high-level" && (
                  <section className="panel system-design">
                    <div className="panel__header">
                      <h3>Layered control plane</h3>
                    </div>
                    <div className="diagram-toolbar">
                      {designFlows.map((flow) => (
                        <button
                          key={flow.id}
                          type="button"
                          className={`chip chip--${flow.tone} design-chip ${designFlow === flow.id ? "design-chip--active" : ""}`}
                          onClick={() => setDesignFlow(flow.id)}
                        >
                          {flow.label}
                        </button>
                      ))}
                    </div>

                    <div className="interactive-diagram" role="img" aria-label="Interactive control plane architecture diagram">
                      <svg key={`arch-${designFlow}`} className="arch-svg" viewBox="0 0 1100 520" preserveAspectRatio="xMidYMid meet">
                        <defs>
                          <marker id="arrow-head" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
                            <path d="M0,0 L8,4 L0,8 z" className="arch-svg__arrow-head" />
                          </marker>
                        </defs>

                        <rect x="80" y="24" width="940" height="90" rx="10" className="arch-svg__layer arch-svg__layer--client" />
                        <text x="102" y="52" className="arch-svg__title">Client layer</text>
                        <text x="102" y="79" className="arch-svg__text">User view · Admin dashboard · Admin ops</text>

                        <line x1="550" y1="116" x2="550" y2="142" className="arch-svg__link arch-svg__link--active" markerEnd="url(#arrow-head)" />

                        <rect x="80" y="144" width="940" height="116" rx="10" className="arch-svg__layer arch-svg__layer--api" />
                        <text x="102" y="172" className="arch-svg__title">Gateway + API surface</text>
                        <text x="102" y="199" className="arch-svg__text mono">{selectedDesignFlow.api}</text>
                        <text x="102" y="224" className="arch-svg__muted">All APIs are listed below this diagram.</text>

                        <line x1="550" y1="262" x2="550" y2="286" className="arch-svg__link arch-svg__link--active" markerEnd="url(#arrow-head)" />

                        <rect x="80" y="288" width="940" height="116" rx="10" className="arch-svg__layer arch-svg__layer--policy" />
                        <text x="102" y="316" className="arch-svg__title">Control plane policy engine</text>
                        <text x="102" y="340" className="arch-svg__text">Hard constraints · class-aware ordering · replica-aware placement</text>
                        <text x="102" y="362" className="arch-svg__text">Priority preemption · reconcile loop · anti-churn guardrails</text>

                        <line x1="550" y1="406" x2="550" y2="432" className="arch-svg__link arch-svg__link--active" markerEnd="url(#arrow-head)" />

                        <rect x="80" y="434" width="940" height="72" rx="10" className="arch-svg__layer arch-svg__layer--store" />
                        <text x="102" y="462" className="arch-svg__title">Persistence + observability (Postgres)</text>
                        <text x="102" y="486" className="arch-svg__text">Atomic state writes -&gt; event log -&gt; telemetry snapshots -&gt; read models</text>
                      </svg>
                    </div>
                    <details className="arch-details">
                      <summary>All supported APIs</summary>
                      <div className="diagram-api-list">
                        {supportedAPIs.map((api) => (
                          <span key={api} className="mono">{api}</span>
                        ))}
                      </div>
                    </details>
                  </section>
                )}

                {designTab === "api-sequences" && (
                  <section className="panel system-design">
                    <div className="panel__header">
                      <h3>API sequence diagrams</h3>
                    </div>

                    <div className="diagram-toolbar">
                      {designFlows.map((flow) => (
                        <button
                          key={flow.id}
                          type="button"
                          className={`chip chip--${flow.tone} design-chip ${designFlow === flow.id ? "design-chip--active" : ""}`}
                          onClick={() => setDesignFlow(flow.id)}
                        >
                          {flow.api}
                        </button>
                      ))}
                    </div>

                    <article className="sequence-card sequence-card--full">
                      <h4>{selectedDesignFlow.api}</h4>
                      <svg key={`seq-${designFlow}`} className="sequence-svg" viewBox="0 0 1100 420" preserveAspectRatio="xMidYMid meet">
                        <defs>
                          <marker id="seq-arrow-head" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
                            <path d="M0,0 L8,4 L0,8 z" className="arch-svg__arrow-head" />
                          </marker>
                        </defs>
                        {sequenceParticipants.map((participant, idx) => {
                          const x = 90 + idx * 230;
                          return (
                            <g key={participant}>
                              <rect x={x - 70} y="18" width="140" height="34" rx="7" className="seq-svg__head" />
                              <text x={x} y="40" textAnchor="middle" className="seq-svg__head-text">{participant}</text>
                              <line x1={x} y1="60" x2={x} y2="390" className="seq-svg__lifeline" />
                            </g>
                          );
                        })}
                        {sequenceByFlow[designFlow].map((step, idx) => {
                          const y = 96 + idx * 56;
                          const fromIdx = Math.max(sequenceParticipants.indexOf(step.from), 0);
                          const toIdx = Math.max(sequenceParticipants.indexOf(step.to), 0);
                          const x1 = 90 + fromIdx * 230;
                          const x2 = 90 + toIdx * 230;
                          const left = Math.min(x1, x2);
                          const width = Math.max(Math.abs(x2 - x1), 2);
                          const textX = left + width / 2;
                          return (
                            <g key={`${step.from}-${step.to}-${idx}`}>
                              <line
                                x1={x1}
                                y1={y}
                                x2={x2}
                                y2={y}
                                className="seq-svg__arrow seq-svg__arrow--active"
                                markerEnd="url(#seq-arrow-head)"
                                style={{ "--step-index": idx } as CSSProperties}
                              />
                              <rect x={textX - 160} y={y - 22} width="320" height="18" rx="5" className="seq-svg__label-bg" />
                              <text x={textX} y={y - 9} textAnchor="middle" className="seq-svg__label">{step.action}</text>
                            </g>
                          );
                        })}
                      </svg>
                      <div className="sequence-note">
                        Every mutating path ends with both event emission and telemetry snapshot persisted through the control plane into Postgres.
                      </div>
                    </article>

                    <article className="sequence-card sequence-card--full">
                      <h4>Policy groups exercised</h4>
                      <div className="diagram-policy-grid">
                        <span>Admission policy: request validation and normalized defaults</span>
                        <span>Scheduling policy: fit, score, priority, preemption</span>
                        <span>Reconcile policy: retry pending and recover node state</span>
                        <span>Audit policy: structured event messages with root-cause hints</span>
                        <span>Telemetry policy: periodic and mutation-driven snapshots</span>
                      </div>
                    </article>
                  </section>
                )}

                {designTab === "limitations" && (
                  <section className="panel system-design">
                    <div className="panel__header">
                      <h3>Current limitations and future paths</h3>
                    </div>
                    <div className="limitations-list">
                      {limitationSections.map((section) => (
                        <details key={section.id} className="limitation-card">
                          <summary className="limitation-card__summary">
                            <strong>{section.title}</strong>
                            <span>Expand</span>
                          </summary>
                          <p>{section.note}</p>
                          {section.refs ? <p className="limitation-card__refs">Code touchpoints: {section.refs}</p> : null}
                          <p className="limitation-card__future">
                            <strong>Future direction:</strong> {section.future}
                          </p>
                        </details>
                      ))}
                    </div>
                  </section>
                )}

                {designTab === "decisions" && (
                  <section className="panel system-design">
                    <div className="panel__header">
                      <h3>Decision-making internals</h3>
                    </div>
                    <div className="diagram-toolbar">
                      <span className="chip chip--success">Class order: inference &gt; training &gt; batch</span>
                      <span className="chip chip--warning">Capacity preference: on-demand &gt; spot</span>
                      <span className="chip chip--neutral">Priority gates first, then class/capacity scoring</span>
                    </div>
                    <article className="sequence-card sequence-card--full">
                      <h4>Scoring summary (high level)</h4>
                      <p className="sequence-line mono">
                        score(node, workload) = priority_fit + class_weight + capacity_weight + placement_efficiency - churn_penalty
                      </p>
                      <p className="sequence-line">
                        Class weight emphasizes latency-sensitive demand: <strong>inference &gt; training &gt; batch</strong>.
                        Capacity weight favors reliability/cost policy: <strong>on-demand &gt; spot</strong> unless workload is explicitly spot-tolerant.
                      </p>
                    </article>
                    <div className="limitations-list">
                      {decisionSections.map((section) => (
                        <details key={section.id} className="limitation-card">
                          <summary className="limitation-card__summary">
                            <strong>{section.title}</strong>
                            <span>Expand</span>
                          </summary>
                          <p>{section.detail}</p>
                          <p className="limitation-card__future">
                            <strong>Current behavior:</strong> {section.implementation}
                          </p>
                        </details>
                      ))}
                    </div>
                  </section>
                )}
              </div>
            )}

          </section>
        </div>
      </div>
    </main>
  );
}
