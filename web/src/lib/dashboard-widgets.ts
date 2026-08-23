export type DashboardWidgetType = "lineplot" | "barplot" | "scatterplot" | "confusion_matrix" | "progress" | "logs" | "gauge";
export type DashboardSourceKind = "metric" | "resource" | "matrix" | "progress" | "log";
export type DashboardSourceRole = "x" | "y";
export type DashboardWidgetSource = { kind: DashboardSourceKind; name: string; role?: DashboardSourceRole };
export type DashboardWidgetSize = { columns: number; rows: number };
export type DashboardWidgetPosition = { x: number; y: number };

export type DashboardWidget = {
  id: string;
  type: DashboardWidgetType;
  title?: string;
  size: DashboardWidgetSize;
  position: DashboardWidgetPosition;
  sources: DashboardWidgetSource[];
  x_axis?: "time" | "step";
  grid_columns?: 4 | 12;
  time_range?: "1h" | "6h" | "24h" | "7d" | "all";
  gauge_max_mode?: "historical" | "fixed";
  gauge_max_value?: number;
};

export type DashboardTemplateCardinality = { min: number; max: number };
export type DashboardTemplateSlot = {
  id: string;
  required_tags?: string[];
  optional_tags?: string[];
  source_types: DashboardSourceKind[];
  cardinality: DashboardTemplateCardinality;
  role?: DashboardSourceRole;
  on_missing?: "error" | "omit_slot" | "omit_widget";
  on_ambiguous?: "error" | "omit_slot" | "omit_widget";
};
export type DashboardTemplateWidget = Omit<DashboardWidget, "sources"> & { slots: DashboardTemplateSlot[] };
export type DashboardTemplate = { id: string; name?: string; description?: string; schema_version: number; version: number; widgets: DashboardTemplateWidget[] };
export type DashboardTemplateSlotResolution = {
  widget_id: string;
  slot_id: string;
  status: "resolved" | "missing" | "ambiguous" | "incompatible";
  candidates: DashboardWidgetSource[];
  selected: DashboardWidgetSource[];
};
export type DashboardTemplateResolution = {
  template_id: string;
  schema_version: number;
  template_version: number;
  attempt_id: string;
  compatibility: DashboardCompatibility;
  fallback_reason?: "unsupported_schema_version" | "unsupported_widget_type";
  widgets: DashboardWidget[];
  widget_results: Array<{ widget_id: string; status: "resolved" | "partial" | "omitted" | "unresolved" }>;
  slot_results: DashboardTemplateSlotResolution[];
};
export type DashboardTemplateOverride = { widget_id: string; slot_id: string; sources: DashboardWidgetSource[] };
export type DashboardCompatibility = "compatible" | "partially_compatible" | "incompatible";
export type DashboardTemplateReference = { template_id: string; template_version: number; schema_version: number };
export type DashboardTemplateMaterialization = DashboardTemplateReference & { applied_at: string };
export type DashboardPreference = { schema_version: number; widgets: DashboardWidget[] | null; compatibility: DashboardCompatibility; materialized_from?: DashboardTemplateMaterialization | null; updated_at?: string; fallback_reason?: "unsupported_schema_version" | "invalid_saved_configuration" | "unsupported_widgets_omitted" };

export const dashboardTemplateSchemaVersion=1;

export type DashboardSources = {
  metrics: string[];
  resources: string[];
  matrices: string[];
  progress: boolean;
  logs?: boolean;
};

export const widgetCatalog: ReadonlyArray<{ type: DashboardWidgetType; label: string; description: string }> = [
  { type: "lineplot", label: "Line plot", description: "A time series rendered as a continuous line." },
  { type: "barplot", label: "Bar plot", description: "Recent observations rendered as discrete bars." },
  { type: "scatterplot", label: "Scatter plot", description: "Observations plotted by step or capture time." },
  { type: "confusion_matrix", label: "Confusion matrix", description: "Absolute or normalized classification outcomes." },
  { type: "progress", label: "Progress", description: "Global progress, current stage and upcoming milestones." },
  { type: "logs", label: "Logs", description: "Live stdout, stderr, or both streams." },
  { type: "gauge", label: "Gauge", description: "Current value against a fixed or historical maximum." },
];

export const dashboardSchemaVersion=1;

export function restoreDashboardWidgets(value:unknown):DashboardWidget[]|null{
  if(!Array.isArray(value)||value.length>64)return null;
  const types=new Set(widgetCatalog.map(item=>item.type)),kinds=new Set<DashboardSourceKind>(["metric","resource","matrix","progress","log"]);
  const widgets:DashboardWidget[]=[];
  for(const candidate of value){
    if(!candidate||typeof candidate!=="object")return null;const item=candidate as Partial<DashboardWidget>;
    if(typeof item.id!=="string"||!item.id||!types.has(item.type as DashboardWidgetType)||!item.size||!item.position||!Array.isArray(item.sources))return null;
    if(item.sources.some(source=>!source||!kinds.has(source.kind)||typeof source.name!=="string"||!source.name))return null;
    const factor=item.grid_columns===12?1:item.grid_columns===4?3:6;
    widgets.push({id:item.id,type:item.type as DashboardWidgetType,title:typeof item.title==="string"?item.title.trim().slice(0,120):undefined,size:{columns:clampColumns(item.size.columns*factor),rows:clampRows(item.size.rows*factor)},position:{x:Math.max(0,Math.min(11,(item.position.x||0)*factor)),y:Math.max(0,(item.position.y||0)*factor)},sources:item.sources.map(source=>({...source})),x_axis:item.x_axis==="step"?"step":"time",grid_columns:12,time_range:validRange(item.time_range)?item.time_range:"all",gauge_max_mode:item.gauge_max_mode==="fixed"?"fixed":"historical",gauge_max_value:typeof item.gauge_max_value==="number"&&Number.isFinite(item.gauge_max_value)&&item.gauge_max_value>0?item.gauge_max_value:undefined});
  }
  if(new Set(widgets.map(widget=>widget.id)).size!==widgets.length)return null;
  return layoutDashboardWidgets(widgets);
}

export function defaultDashboardWidgets(sources: DashboardSources): DashboardWidget[] {
  const definitions: Array<{ type: DashboardWidgetType; source: DashboardWidgetSource }> = [];
  if (sources.progress) definitions.push({ type: "progress", source: { kind: "progress", name: "progress" } });
  for (const name of sources.metrics) definitions.push({ type: "lineplot", source: { kind: "metric", name } });
  for (const name of sources.resources) definitions.push({ type: "lineplot", source: { kind: "resource", name } });
  for (const name of sources.matrices) definitions.push({ type: "confusion_matrix", source: { kind: "matrix", name } });
  return layoutDashboardWidgets(definitions.map(definition => ({
    id: `default-${definition.source.kind}-${safeID(definition.source.name)}`,
    type: definition.type,
    size: { columns: definition.type === "progress" ? 12 : 6, rows: 3 },
    position: { x: 0, y: 0 },
    sources: [definition.source],
    x_axis: "time",
    grid_columns:12,time_range:"all",
  })) as DashboardWidget[]);
}

export function createDashboardWidget(type: DashboardWidgetType, sources: DashboardSources, id = newWidgetID()): DashboardWidget {
  const source = firstCompatibleSource(type, sources);
  const numeric=[...sources.metrics.map(name=>({kind:"metric" as const,name})),...sources.resources.map(name=>({kind:"resource" as const,name}))];
  return {
    id,
    type,
    size: { columns: type === "progress" || type === "logs" ? 12 : 6, rows: type === "logs" ? 6 : 3 },
    position: { x: 0, y: Number.MAX_SAFE_INTEGER },
    sources: type==="logs"?[{kind:"log",name:"stdout"}]:type==="scatterplot"&&numeric.length?[{...numeric[0],role:"x"},{...(numeric[1]??numeric[0]),role:"y"}]:source ? [source] : [],
    x_axis: "time",
    grid_columns:12,time_range:"all",gauge_max_mode:"historical",
  };
}

export function removeDashboardWidget(widgets: DashboardWidget[], id: string) {
  return layoutDashboardWidgets(widgets.filter(widget => widget.id !== id));
}

export function layoutDashboardWidgets(widgets: DashboardWidget[]) {
  const occupied = new Set<string>();
  return widgets.map(widget => {
    const columns = clampColumns(widget.size.columns), rows = clampRows(widget.size.rows);
    let position = { x: 0, y: 0 };
    placement: for (let y = 0; ; y++) {
      for (let x = 0; x <= 12 - columns; x++) {
        if (fits(occupied, x, y, columns, rows)) { position = { x, y }; break placement; }
      }
    }
    occupy(occupied, position.x, position.y, columns, rows);
    return {...widget,size:{columns,rows},position,grid_columns:12 as const};
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
  return layoutDashboardWidgets(widgets.map(widget=>widget.id===id?{...widget,size:{columns:clampColumns(size.columns),rows:clampRows(size.rows)}}:widget));
}

export function compatibleSourceKinds(type: DashboardWidgetType): DashboardSourceKind[] {
  if (type === "confusion_matrix") return ["matrix"];
  if (type === "progress") return ["progress"];
  if(type === "logs") return ["log"];
  return ["metric", "resource"];
}

function firstCompatibleSource(type: DashboardWidgetType, sources: DashboardSources): DashboardWidgetSource | undefined {
  if (type === "progress") return sources.progress ? { kind: "progress", name: "progress" } : undefined;
  if(type === "logs")return sources.logs?{kind:"log",name:"stdout"}:undefined;
  if (type === "confusion_matrix") return sources.matrices[0] ? { kind: "matrix", name: sources.matrices[0] } : undefined;
  if (sources.metrics[0]) return { kind: "metric", name: sources.metrics[0] };
  if (sources.resources[0]) return { kind: "resource", name: sources.resources[0] };
  return undefined;
}

function safeID(value: string) { return value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || "source"; }
function newWidgetID() { return typeof crypto.randomUUID === "function" ? crypto.randomUUID() : `widget-${Date.now()}-${Math.random().toString(16).slice(2)}`; }
function clampColumns(value:number){return Math.max(1,Math.min(12,Math.round(value)||1))}
function clampRows(value:number){return Math.max(1,Math.min(12,Math.round(value)||1))}
function validRange(value:unknown):value is DashboardWidget["time_range"]{return ["1h","6h","24h","7d","all"].includes(String(value))}
function fits(occupied:Set<string>,x:number,y:number,columns:number,rows:number){for(let row=y;row<y+rows;row++)for(let column=x;column<x+columns;column++)if(occupied.has(`${column}:${row}`))return false;return true}
function occupy(occupied:Set<string>,x:number,y:number,columns:number,rows:number){for(let row=y;row<y+rows;row++)for(let column=x;column<x+columns;column++)occupied.add(`${column}:${row}`)}
