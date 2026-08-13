export type DashboardWidgetType = "lineplot" | "barplot" | "scatterplot" | "confusion_matrix" | "progress";
export type DashboardSourceKind = "metric" | "resource" | "matrix" | "progress";
export type DashboardWidgetSource = { kind: DashboardSourceKind; name: string };
export type DashboardWidgetSize = { columns: 1 | 2; rows: 1 | 2 };
export type DashboardWidgetPosition = { x: number; y: number };

export type DashboardWidget = {
  id: string;
  type: DashboardWidgetType;
  size: DashboardWidgetSize;
  position: DashboardWidgetPosition;
  sources: DashboardWidgetSource[];
};

export type DashboardSources = {
  metrics: string[];
  resources: string[];
  matrices: string[];
  progress: boolean;
};

export const widgetCatalog: ReadonlyArray<{ type: DashboardWidgetType; label: string; description: string }> = [
  { type: "lineplot", label: "Line plot", description: "A time series rendered as a continuous line." },
  { type: "barplot", label: "Bar plot", description: "Recent observations rendered as discrete bars." },
  { type: "scatterplot", label: "Scatter plot", description: "Observations plotted by step or capture time." },
  { type: "confusion_matrix", label: "Confusion matrix", description: "Absolute or normalized classification outcomes." },
  { type: "progress", label: "Progress", description: "Global progress, current stage and upcoming milestones." },
];

export function defaultDashboardWidgets(sources: DashboardSources): DashboardWidget[] {
  const definitions: Array<{ type: DashboardWidgetType; source: DashboardWidgetSource }> = [];
  if (sources.progress) definitions.push({ type: "progress", source: { kind: "progress", name: "progress" } });
  for (const name of sources.metrics) definitions.push({ type: "lineplot", source: { kind: "metric", name } });
  for (const name of sources.resources) definitions.push({ type: "lineplot", source: { kind: "resource", name } });
  for (const name of sources.matrices) definitions.push({ type: "confusion_matrix", source: { kind: "matrix", name } });
  return layoutDashboardWidgets(definitions.map(definition => ({
    id: `default-${definition.source.kind}-${safeID(definition.source.name)}`,
    type: definition.type,
    size: { columns: definition.type === "progress" ? 2 : 1, rows: 1 },
    position: { x: 0, y: 0 },
    sources: [definition.source],
  })) as DashboardWidget[]);
}

export function createDashboardWidget(type: DashboardWidgetType, sources: DashboardSources, id = newWidgetID()): DashboardWidget {
  const source = firstCompatibleSource(type, sources);
  return {
    id,
    type,
    size: { columns: type === "progress" ? 2 : 1, rows: 1 },
    position: { x: 0, y: Number.MAX_SAFE_INTEGER },
    sources: source ? [source] : [],
  };
}

export function removeDashboardWidget(widgets: DashboardWidget[], id: string) {
  return layoutDashboardWidgets(widgets.filter(widget => widget.id !== id));
}

export function layoutDashboardWidgets(widgets: DashboardWidget[]) {
  let x=0,y=0;
  return widgets.map(widget=>{
    if(widget.size.columns===2&&x!==0){x=0;y+=1}
    const positioned={...widget,position:{x,y}};
    if(widget.size.columns===2){x=0;y+=widget.size.rows}else{x+=1;if(x===2){x=0;y+=widget.size.rows}}
    return positioned;
  });
}

export function compatibleSourceKinds(type: DashboardWidgetType): DashboardSourceKind[] {
  if (type === "confusion_matrix") return ["matrix"];
  if (type === "progress") return ["progress"];
  return ["metric", "resource"];
}

function firstCompatibleSource(type: DashboardWidgetType, sources: DashboardSources): DashboardWidgetSource | undefined {
  if (type === "progress") return sources.progress ? { kind: "progress", name: "progress" } : undefined;
  if (type === "confusion_matrix") return sources.matrices[0] ? { kind: "matrix", name: sources.matrices[0] } : undefined;
  if (sources.metrics[0]) return { kind: "metric", name: sources.metrics[0] };
  if (sources.resources[0]) return { kind: "resource", name: sources.resources[0] };
  return undefined;
}

function safeID(value: string) { return value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || "source"; }
function newWidgetID() { return typeof crypto.randomUUID === "function" ? crypto.randomUUID() : `widget-${Date.now()}-${Math.random().toString(16).slice(2)}`; }
