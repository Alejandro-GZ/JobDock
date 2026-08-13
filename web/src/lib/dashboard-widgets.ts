export type DashboardWidgetType = "lineplot" | "barplot" | "scatterplot" | "confusion_matrix" | "progress";
export type DashboardSourceKind = "metric" | "resource" | "matrix" | "progress";
export type DashboardSourceRole = "x" | "y";
export type DashboardWidgetSource = { kind: DashboardSourceKind; name: string; role?: DashboardSourceRole };
export type DashboardWidgetSize = { columns: 1 | 2; rows: 1 | 2 };
export type DashboardWidgetPosition = { x: number; y: number };

export type DashboardWidget = {
  id: string;
  type: DashboardWidgetType;
  size: DashboardWidgetSize;
  position: DashboardWidgetPosition;
  sources: DashboardWidgetSource[];
  x_axis?: "time" | "step";
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
    x_axis: "time",
  })) as DashboardWidget[]);
}

export function createDashboardWidget(type: DashboardWidgetType, sources: DashboardSources, id = newWidgetID()): DashboardWidget {
  const source = firstCompatibleSource(type, sources);
  const numeric=[...sources.metrics.map(name=>({kind:"metric" as const,name})),...sources.resources.map(name=>({kind:"resource" as const,name}))];
  return {
    id,
    type,
    size: { columns: type === "progress" ? 2 : 1, rows: 1 },
    position: { x: 0, y: Number.MAX_SAFE_INTEGER },
    sources: type==="scatterplot"&&numeric.length?[{...numeric[0],role:"x"},{...(numeric[1]??numeric[0]),role:"y"}]:source ? [source] : [],
    x_axis: "time",
  };
}

export function removeDashboardWidget(widgets: DashboardWidget[], id: string) {
  return layoutDashboardWidgets(widgets.filter(widget => widget.id !== id));
}

export function layoutDashboardWidgets(widgets: DashboardWidget[]) {
  const occupied = new Set<string>();
  return widgets.map(widget => {
    const columns = clampSize(widget.size.columns), rows = clampSize(widget.size.rows);
    let position = { x: 0, y: 0 };
    placement: for (let y = 0; ; y++) {
      for (let x = 0; x <= 2 - columns; x++) {
        if (fits(occupied, x, y, columns, rows)) { position = { x, y }; break placement; }
      }
    }
    occupy(occupied, position.x, position.y, columns, rows);
    return {...widget,size:{columns,rows},position};
  });
}

export function moveDashboardWidget(widgets: DashboardWidget[], id: string, target: string | "earlier" | "later") {
  const ordered = [...widgets].sort((a,b)=>a.position.y-b.position.y||a.position.x-b.position.x);
  const from = ordered.findIndex(widget=>widget.id===id);
  const to = target === "earlier" ? from-1 : target === "later" ? from+1 : ordered.findIndex(widget=>widget.id===target);
  if (from < 0 || to < 0 || to >= ordered.length || from === to) return layoutDashboardWidgets(ordered);
  const [item] = ordered.splice(from,1);
  ordered.splice(to,0,item);
  return layoutDashboardWidgets(ordered);
}

export function resizeDashboardWidget(widgets: DashboardWidget[], id: string, size: DashboardWidgetSize) {
  return layoutDashboardWidgets(widgets.map(widget=>widget.id===id?{...widget,size:{columns:clampSize(size.columns),rows:clampSize(size.rows)}}:widget));
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
function clampSize(value:number):1|2{return value>=2?2:1}
function fits(occupied:Set<string>,x:number,y:number,columns:number,rows:number){for(let row=y;row<y+rows;row++)for(let column=x;column<x+columns;column++)if(occupied.has(`${column}:${row}`))return false;return true}
function occupy(occupied:Set<string>,x:number,y:number,columns:number,rows:number){for(let row=y;row<y+rows;row++)for(let column=x;column<x+columns;column++)occupied.add(`${column}:${row}`)}
