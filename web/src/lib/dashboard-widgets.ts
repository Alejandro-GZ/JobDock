export type DashboardWidgetType = "lineplot" | "loss_curve" | "learning_curve" | "anomaly_timeline" | "feature_importance" | "shap_summary" | "partial_dependence" | "embedding_scatter" | "cluster_scatter" | "barplot" | "area_chart" | "stacked_bar" | "scatterplot" | "starplot" | "histogram" | "boxplot" | "violin" | "heatmap" | "correlation_heatmap" | "confusion_matrix" | "data_grid" | "roc_curve" | "precision_recall_curve" | "calibration_curve" | "prediction_vs_actual" | "residual_plot" | "bubble_chart" | "parallel_coordinates" | "pie_chart" | "donut_chart" | "treemap" | "waterfall" | "progress" | "logs" | "kpi" | "gauge";
export type ScalarAggregation = "last" | "min" | "max" | "avg";
export type DashboardSourceKind = "metric" | "resource" | "distribution" | "matrix" | "table" | "progress" | "log";
export type DashboardSourceRole = "x" | "y" | "size" | "color" | "category" | "value" | "id" | "parent" | "kind";
export type DashboardWidgetSource = { kind: DashboardSourceKind; name: string; role?: DashboardSourceRole };
export type DashboardWidgetSize = { columns: number; rows: number };
export type DashboardWidgetPosition = { x: number; y: number };
export type DashboardAxisAppearance = {
  label?: string;
  unit?: string;
  scale?: "linear" | "log";
  range?: "auto" | "manual";
  min?: number;
  max?: number;
};
export type DashboardSeriesAppearance = { label?: string; unit?: string; color?: string; normalization?: "historical"|"manual"|"zero_to_one"; min?:number;max?:number };
export type DashboardWidgetAppearance = {
  schema_version: 1;
  subtitle?: string;
  color_scheme?: "default" | "cool" | "warm" | "monochrome";
  series?: Record<string, DashboardSeriesAppearance>;
  legend?: "auto" | "hidden" | "open";
  show_grid?: boolean;
  x_axis?: DashboardAxisAppearance;
  y_axis?: DashboardAxisAppearance;
  line_style?: "solid" | "dashed" | "dotted";
  line_width?: number;
  show_points?: boolean;
  point_size?: number;
  opacity?: number;
  matrix_mode?: "absolute" | "normalized";
  stack_mode?: "overlap" | "stacked";
  heatmap_scale?: "auto" | "manual";
  heatmap_min?: number;
  heatmap_max?: number;
  heatmap_palette?: "sequential" | "diverging";
};

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
  histogram_bins?: number;
  gauge_max_mode?: "historical" | "fixed";
  gauge_max_value?: number;
  scalar_aggregation?: ScalarAggregation;
  gauge_style?: "gauge" | "bullet";
  target_value?: number;
  warning_value?: number;
  critical_value?: number;
  domain_min?: number;
  domain_max?: number;
  threshold_direction?: "higher_is_worse" | "lower_is_worse";
  show_delta?: boolean;
  table_columns?: string[];
  table_sort_by?: string;
  table_sort_order?: "asc"|"desc";
  table_page_size?: number;
  normalize_axes?: boolean;
  category_limit?: number;
  group_small_categories?: boolean;
  residual_x_axis?: "prediction"|"actual";
  feature_importance_top_n?: number;
  feature_importance_absolute?: boolean;
  shap_mode?: "beeswarm"|"bar";
  projection_color_by?: "label"|"cluster"|"color";
  appearance?: DashboardWidgetAppearance;
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
export type DashboardTemplateCategory = "general"|"classification"|"regression"|"clustering-representation"|"computer-vision"|"nlp-speech"|"generative-ai"|"ranking-recommendation"|"time-series-anomaly"|"reinforcement-learning"|"hpo-model-selection";
export type DashboardTemplate = { id: string; name?: string; description?: string; category?: DashboardTemplateCategory; schema_version: number; version: number; widgets: DashboardTemplateWidget[] };
export type DashboardTemplateMatch = {template_id:string;compatibility:DashboardCompatibility;applicable:boolean;missing_required:number;ambiguous_sources:number};
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
  fallback_reason?: "unsupported_schema_version" | "unsupported_widget_type" | "unsupported_widget_appearance_version";
  widgets: DashboardWidget[];
  widget_results: Array<{ widget_id: string; status: "resolved" | "partial" | "omitted" | "unresolved" }>;
  slot_results: DashboardTemplateSlotResolution[];
};
export type DashboardTemplateOverride = { widget_id: string; slot_id: string; sources: DashboardWidgetSource[] };
export type DashboardCompatibility = "compatible" | "partially_compatible" | "incompatible";
export type DashboardTemplateReference = { template_id: string; template_version: number; schema_version: number };
export type DashboardTemplateMaterialization = DashboardTemplateReference & { applied_at: string };
export type DashboardSummary = {id:string;name:string;sort_order:number;is_default:boolean;created_at:string;updated_at:string};
export type DashboardList = {items:DashboardSummary[];active_dashboard_id:string;default_dashboard_id:string};
export type DashboardPreference = DashboardSummary & { schema_version: number; widgets: DashboardWidget[] | null; compatibility: DashboardCompatibility; materialized_from?: DashboardTemplateMaterialization | null; fallback_reason?: "unsupported_schema_version" | "invalid_saved_configuration" | "unsupported_widgets_omitted" | "unsupported_widget_appearance_omitted" };

export const dashboardTemplateSchemaVersion=1;

export type DashboardSources = {
  metrics: string[];
  resources: string[];
  matrices: string[];
  matrix_types?: Record<string,string>;
  distributions?: string[];
  tables?: string[];
  table_types?: Record<string,string>;
  progress: boolean;
  logs?: boolean;
};

export type WidgetCatalogCategory="trends"|"relationships"|"summaries"|"diagnostics"|"operational";
export const widgetCategoryLabels:Readonly<Record<WidgetCatalogCategory,string>>={trends:"Trends",relationships:"Relationships",summaries:"Summaries",diagnostics:"Diagnostics",operational:"Operational"};
export const widgetCatalog: ReadonlyArray<{ type: DashboardWidgetType; label: string; description: string;category:WidgetCatalogCategory }> = [
  { type: "lineplot", label: "Line plot", description: "A time series rendered as a continuous line.",category:"trends" },
  { type: "loss_curve", label: "Loss curve", description: "Semantically tagged train and validation loss over step, epoch, or time.",category:"trends" },
  { type: "learning_curve", label: "Learning curve", description: "Train and validation score or error over declared training progress.",category:"trends" },
  { type: "anomaly_timeline", label: "Anomaly score", description: "Reported anomaly scores, thresholds, and detection markers over time or step.",category:"trends" },
  { type: "barplot", label: "Bar plot", description: "Recent observations rendered as discrete bars.",category:"trends" },
  { type: "area_chart", label: "Area chart", description: "One or more compatible series rendered as overlapping or stacked areas.",category:"trends" },
  { type: "stacked_bar", label: "Stacked bar chart", description: "Shared categories composed from multiple ordered series.",category:"trends" },
  { type: "scatterplot", label: "Scatter plot", description: "Observations plotted by step or capture time.",category:"relationships" },
  { type: "starplot", label: "STAR plot", description: "Latest values compared across normalized radial axes.",category:"relationships" },
  { type: "histogram", label: "Histogram", description: "Compare bounded feature, error, or drift distributions.",category:"diagnostics" },
  { type: "boxplot", label: "Box plot", description: "Compare quartiles, whiskers, and bounded outliers by group.",category:"diagnostics" },
  { type: "violin", label: "Violin plot", description: "Compare compact density summaries by group.",category:"diagnostics" },
  { type: "heatmap", label: "Heatmap", description: "Rectangular typed values with optional row and column labels.",category:"diagnostics" },
  { type: "correlation_heatmap", label: "Correlation heatmap", description: "Declared variable correlations on a preserved symmetric matrix.",category:"diagnostics" },
  { type: "confusion_matrix", label: "Confusion matrix", description: "Absolute or normalized classification outcomes.",category:"diagnostics" },
  { type: "data_grid", label: "Table / Data grid", description: "Paginated typed records with server-side sort and filters.",category:"operational" },
  { type: "roc_curve", label: "ROC curve", description: "False-positive and true-positive rates with optional thresholds and AUC.",category:"diagnostics" },
  { type: "precision_recall_curve", label: "Precision–Recall curve", description: "Recall and precision with optional thresholds and average precision.",category:"diagnostics" },
  { type: "calibration_curve", label: "Calibration curve", description: "Predicted probability against observed fraction by bin.",category:"diagnostics" },
  { type: "prediction_vs_actual", label: "Prediction vs Actual", description: "Reported regression pairs against the ideal y = x reference.",category:"diagnostics" },
  { type: "residual_plot", label: "Residual plot", description: "Reported regression residuals against prediction or actual values.",category:"diagnostics" },
  { type: "feature_importance", label: "Feature importance", description: "Reported signed feature contributions with method and model provenance.",category:"diagnostics" },
  { type: "shap_summary", label: "SHAP summary", description: "Bounded precomputed SHAP effects as a beeswarm or global importance view.",category:"diagnostics" },
  { type: "partial_dependence", label: "Partial dependence", description: "A precomputed one-dimensional feature response with optional reported range.",category:"diagnostics" },
  { type: "embedding_scatter", label: "Embedding scatter", description: "A bounded precomputed 2D/3D latent projection.",category:"relationships" },
  { type: "cluster_scatter", label: "Cluster scatter", description: "A bounded projection grouped by reported cluster or label.",category:"relationships" },
  { type: "bubble_chart", label: "Bubble chart", description: "Three numeric dimensions with optional grouping.",category:"relationships" },
  { type: "parallel_coordinates", label: "Parallel coordinates", description: "Compare multiple typed dimensions from bounded tabular rows.",category:"relationships" },
  { type: "pie_chart", label: "Pie chart", description: "Bounded categorical proportions with explicit Other grouping.",category:"summaries" },
  { type: "donut_chart", label: "Donut chart", description: "Bounded categorical proportions in a compact ring.",category:"summaries" },
  { type: "treemap", label: "Treemap", description: "Hierarchical non-negative values reported by the job.",category:"relationships" },
  { type: "waterfall", label: "Waterfall", description: "Ordered contributions and explicit totals or subtotals.",category:"summaries" },
  { type: "progress", label: "Progress", description: "Global progress, current stage and upcoming milestones.",category:"summaries" },
  { type: "logs", label: "Logs", description: "Live stdout, stderr, or both streams.",category:"operational" },
  { type: "kpi", label: "KPI / Scorecard", description: "A scalar summary with optional delta and threshold state.",category:"summaries" },
  { type: "gauge", label: "Gauge / Bullet", description: "A scalar value against a target, thresholds, and configurable domain.",category:"summaries" },
];

export const dashboardSchemaVersion=1;

export function restoreDashboardWidgets(value:unknown):DashboardWidget[]|null{
  if(!Array.isArray(value)||value.length>64)return null;
  const types=new Set(widgetCatalog.map(item=>item.type)),kinds=new Set<DashboardSourceKind>(["metric","resource","distribution","matrix","table","progress","log"]);
  const widgets:DashboardWidget[]=[];
  for(const candidate of value){
    if(!candidate||typeof candidate!=="object")return null;const item=candidate as Partial<DashboardWidget>;
    if(typeof item.id!=="string"||!item.id||!types.has(item.type as DashboardWidgetType)||!item.size||!item.position||!Array.isArray(item.sources))return null;
    if(item.sources.some(source=>!source||!kinds.has(source.kind)||typeof source.name!=="string"||!source.name))return null;
    const factor=item.grid_columns===12?1:item.grid_columns===4?3:6;
    const scalar=item.type==="kpi"||item.type==="gauge",tabular=compatibleSourceKinds(item.type as DashboardWidgetType).includes("table"),categorical=item.type==="pie_chart"||item.type==="donut_chart";
    widgets.push({
      id:item.id,type:item.type as DashboardWidgetType,title:typeof item.title==="string"?item.title.trim().slice(0,120):undefined,
      size:{columns:clampColumns(item.size.columns*factor),rows:clampRows(item.size.rows*factor)},position:{x:Math.max(0,Math.min(11,(item.position.x||0)*factor)),y:Math.max(0,(item.position.y||0)*factor)},sources:item.sources.map(source=>({...source})),x_axis:item.x_axis==="step"?"step":"time",grid_columns:12,time_range:validRange(item.time_range)?item.time_range:"all",
      ...(item.type==="histogram"&&finiteBetween(item.histogram_bins,2,256)?{histogram_bins:Math.round(item.histogram_bins!)}:{}),
      ...(item.type==="gauge"?{gauge_max_mode:item.gauge_max_mode==="fixed"&&finite(item.gauge_max_value)&&item.gauge_max_value!>0?"fixed":"historical",gauge_max_value:finite(item.gauge_max_value)&&item.gauge_max_value!>0?item.gauge_max_value:undefined,gauge_style:item.gauge_style==="bullet"?"bullet":"gauge"}:{}),
      ...(scalar?{scalar_aggregation:["last","min","max","avg"].includes(String(item.scalar_aggregation))?item.scalar_aggregation:"last",target_value:finite(item.target_value)?item.target_value:undefined,warning_value:finite(item.warning_value)?item.warning_value:undefined,critical_value:finite(item.critical_value)?item.critical_value:undefined,domain_min:finite(item.domain_min)?item.domain_min:undefined,domain_max:finite(item.domain_max)?item.domain_max:item.type==="gauge"&&finite(item.gauge_max_value)?item.gauge_max_value:undefined,threshold_direction:item.threshold_direction==="lower_is_worse"?"lower_is_worse":"higher_is_worse",show_delta:item.type==="kpi"&&item.show_delta===true||undefined}:{}),
      ...(tabular?{table_columns:Array.isArray(item.table_columns)?item.table_columns.filter(value=>typeof value==="string").slice(0,64):undefined,table_sort_by:typeof item.table_sort_by==="string"?item.table_sort_by:undefined,table_sort_order:item.table_sort_order==="desc"?"desc":item.table_sort_order==="asc"?"asc":undefined,table_page_size:finiteBetween(item.table_page_size,1,500)?Math.round(item.table_page_size!):undefined}:{}),
      ...(item.type==="parallel_coordinates"&&typeof item.normalize_axes==="boolean"?{normalize_axes:item.normalize_axes}:{}),
      ...(categorical?{category_limit:finiteBetween(item.category_limit,2,64)?Math.round(item.category_limit!):undefined,group_small_categories:typeof item.group_small_categories==="boolean"?item.group_small_categories:undefined}:{}),
      ...(item.type==="residual_plot"?{residual_x_axis:item.residual_x_axis==="actual"?"actual":"prediction"}:{}),
      ...(item.type==="feature_importance"?{feature_importance_top_n:finiteBetween(item.feature_importance_top_n,1,100)?Math.round(item.feature_importance_top_n!):undefined,feature_importance_absolute:item.feature_importance_absolute===true||undefined}:{}),
      ...(item.type==="shap_summary"?{shap_mode:item.shap_mode==="bar"?"bar":"beeswarm"}:{}),
      ...(item.type==="embedding_scatter"||item.type==="cluster_scatter"?{projection_color_by:item.projection_color_by==="cluster"?"cluster":item.projection_color_by==="color"?"color":"label"}:{}),
      appearance:restoreAppearance(item.type as DashboardWidgetType,item.appearance),
    });
  }
  if(new Set(widgets.map(widget=>widget.id)).size!==widgets.length)return null;
  return layoutDashboardWidgets(widgets);
}

export function defaultDashboardWidgets(sources: DashboardSources): DashboardWidget[] {
  const definitions: Array<{ type: DashboardWidgetType; source: DashboardWidgetSource }> = [];
  if (sources.progress) definitions.push({ type: "progress", source: { kind: "progress", name: "progress" } });
  for (const name of sources.metrics) definitions.push({ type: "lineplot", source: { kind: "metric", name } });
  for (const name of sources.resources) definitions.push({ type: "lineplot", source: { kind: "resource", name } });
  for (const name of sources.matrices) definitions.push({ type: sources.matrix_types?.[name]==="correlation"?"correlation_heatmap":sources.matrix_types?.[name]==="heatmap"?"heatmap":"confusion_matrix", source: { kind: "matrix", name } });
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
  const scalar=type==="kpi"||type==="gauge";
  return {
    id,
    type,
    size: { columns: type === "progress" || type === "logs" ? 12 : type==="kpi"?3:6, rows: type === "logs" ? 6 : type==="kpi"?2:3 },
    position: { x: 0, y: Number.MAX_SAFE_INTEGER },
    sources: type==="logs"?[{kind:"log",name:"stdout"}]:type==="scatterplot"&&numeric.length?[{...numeric[0],role:"x"},{...(numeric[1]??numeric[0]),role:"y"}]:type==="starplot"?numeric.slice(0,Math.min(5,numeric.length)):type==="histogram"||type==="boxplot"||type==="violin"?(sources.distributions??[]).slice(0,16).map(name=>({kind:"distribution" as const,name})):source ? [source] : [],
    x_axis: "time",
    grid_columns:12,time_range:"all",
    ...(scalar?{scalar_aggregation:"last" as const,threshold_direction:"higher_is_worse" as const}:{}),
    ...(type==="gauge"?{gauge_max_mode:"historical" as const,gauge_style:"gauge" as const}:{}),
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
	if(type==="loss_curve"||type==="learning_curve"||type==="anomaly_timeline")return ["metric"];
  if (type === "confusion_matrix" || type === "heatmap" || type === "correlation_heatmap") return ["matrix"];
  if (type === "progress") return ["progress"];
  if(type === "logs") return ["log"];
  if(type==="histogram"||type==="boxplot"||type==="violin")return ["distribution"];
  if(["data_grid","roc_curve","precision_recall_curve","calibration_curve","prediction_vs_actual","residual_plot","feature_importance","shap_summary","partial_dependence","embedding_scatter","cluster_scatter","bubble_chart","parallel_coordinates","pie_chart","donut_chart","treemap","waterfall"].includes(type))return ["table"];
  return ["metric", "resource"];
}

function firstCompatibleSource(type: DashboardWidgetType, sources: DashboardSources): DashboardWidgetSource | undefined {
  if (type === "progress") return sources.progress ? { kind: "progress", name: "progress" } : undefined;
  if(type === "logs")return sources.logs?{kind:"log",name:"stdout"}:undefined;
  if(type==="histogram"||type==="boxplot"||type==="violin")return sources.distributions?.[0]?{kind:"distribution",name:sources.distributions[0]}:undefined;
  if(["data_grid","bubble_chart","parallel_coordinates","pie_chart","donut_chart","treemap","waterfall"].includes(type))return sources.tables?.[0]?{kind:"table",name:sources.tables[0]}:undefined;
  if(type==="roc_curve"||type==="precision_recall_curve"||type==="calibration_curve"){const expected=type==="roc_curve"?"roc":type==="precision_recall_curve"?"precision_recall":"calibration";const name=sources.tables?.find(item=>sources.table_types?.[item]===expected);return name?{kind:"table",name}:undefined}
  if(type==="prediction_vs_actual"||type==="residual_plot"){const name=sources.tables?.find(item=>sources.table_types?.[item]==="regression_diagnostics");return name?{kind:"table",name}:undefined}
  if(type==="feature_importance"){const name=sources.tables?.find(item=>sources.table_types?.[item]==="feature_importance");return name?{kind:"table",name}:undefined}
  if(type==="shap_summary"){const name=sources.tables?.find(item=>sources.table_types?.[item]==="shap_attribution");return name?{kind:"table",name}:undefined}
  if(type==="partial_dependence"){const name=sources.tables?.find(item=>sources.table_types?.[item]==="partial_dependence");return name?{kind:"table",name}:undefined}
  if(type==="embedding_scatter"||type==="cluster_scatter"){const name=sources.tables?.find(item=>sources.table_types?.[item]==="projection");return name?{kind:"table",name}:undefined}
  if (type === "confusion_matrix" || type === "heatmap" || type === "correlation_heatmap") {const expected=type==="correlation_heatmap"?"correlation":type;const name=sources.matrices.find(item=>(sources.matrix_types?.[item]??"confusion_matrix")===expected);return name?{kind:"matrix",name}:undefined}
  if (sources.metrics[0]) return { kind: "metric", name: sources.metrics[0] };
  if (sources.resources[0]) return { kind: "resource", name: sources.resources[0] };
  return undefined;
}

function safeID(value: string) { return value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "") || "source"; }
function newWidgetID() { return typeof crypto.randomUUID === "function" ? crypto.randomUUID() : `widget-${Date.now()}-${Math.random().toString(16).slice(2)}`; }
function clampColumns(value:number){return Math.max(1,Math.min(12,Math.round(value)||1))}
function clampRows(value:number){return Math.max(1,Math.min(12,Math.round(value)||1))}
function validRange(value:unknown):value is DashboardWidget["time_range"]{return ["1h","6h","24h","7d","all"].includes(String(value))}
function restoreAppearance(type:DashboardWidgetType,value:unknown):DashboardWidgetAppearance|undefined{
  if(!value||typeof value!=="object")return undefined;
  const item=value as Partial<DashboardWidgetAppearance>;
  if(item.schema_version!==1)return undefined;
  const plot=type==="lineplot"||type==="loss_curve"||type==="learning_curve"||type==="anomaly_timeline"||type==="barplot"||type==="area_chart"||type==="stacked_bar"||type==="scatterplot"||type==="starplot"||type==="histogram"||type==="boxplot"||type==="violin",appearance:DashboardWidgetAppearance={schema_version:1};
  if(plot&&typeof item.subtitle==="string"&&item.subtitle.trim())appearance.subtitle=item.subtitle.trim().slice(0,160);
  if(plot&&["default","cool","warm","monochrome"].includes(String(item.color_scheme)))appearance.color_scheme=item.color_scheme;
  if(plot&&["auto","hidden","open"].includes(String(item.legend)))appearance.legend=item.legend;
  if(plot&&typeof item.show_grid==="boolean")appearance.show_grid=item.show_grid;
  if(plot){const x=restoreAxis(item.x_axis),y=restoreAxis(item.y_axis),series=restoreSeries(item.series,type==="starplot");if(x&&type!=="starplot")appearance.x_axis=x;if(y&&type!=="starplot")appearance.y_axis=y;if(series)appearance.series=series}
  if((type==="lineplot"||type==="loss_curve"||type==="learning_curve")&&["solid","dashed","dotted"].includes(String(item.line_style)))appearance.line_style=item.line_style;
  if((type==="lineplot"||type==="loss_curve"||type==="learning_curve")&&finiteBetween(item.line_width,.5,12))appearance.line_width=item.line_width;
  if((type==="lineplot"||type==="loss_curve"||type==="learning_curve"||type==="scatterplot")&&typeof item.show_points==="boolean")appearance.show_points=item.show_points;
  if((type==="lineplot"||type==="loss_curve"||type==="learning_curve"||type==="scatterplot")&&finiteBetween(item.point_size,1,16))appearance.point_size=item.point_size;
  if(plot&&finiteBetween(item.opacity,.05,1))appearance.opacity=item.opacity;
  if(type==="confusion_matrix"&&(item.matrix_mode==="absolute"||item.matrix_mode==="normalized"))appearance.matrix_mode=item.matrix_mode;
  if(type==="area_chart"&&(item.stack_mode==="overlap"||item.stack_mode==="stacked"))appearance.stack_mode=item.stack_mode;
  if((type==="heatmap"||type==="correlation_heatmap")&&(item.heatmap_scale==="auto"||item.heatmap_scale==="manual"))appearance.heatmap_scale=item.heatmap_scale;
  if((type==="heatmap"||type==="correlation_heatmap")&&(item.heatmap_palette==="sequential"||item.heatmap_palette==="diverging"))appearance.heatmap_palette=item.heatmap_palette;
  if((type==="heatmap"||type==="correlation_heatmap")&&item.heatmap_scale==="manual"&&finite(item.heatmap_min)&&finite(item.heatmap_max)&&item.heatmap_min!<item.heatmap_max!){appearance.heatmap_min=item.heatmap_min;appearance.heatmap_max=item.heatmap_max}
  return appearance;
}
function restoreAxis(value:unknown):DashboardAxisAppearance|undefined{if(!value||typeof value!=="object")return undefined;const item=value as DashboardAxisAppearance,result:DashboardAxisAppearance={};if(typeof item.label==="string"&&item.label.trim())result.label=item.label.trim().slice(0,80);if(typeof item.unit==="string"&&item.unit.trim())result.unit=item.unit.trim().slice(0,64);if(item.scale==="linear"||item.scale==="log")result.scale=item.scale;if(item.range==="auto")result.range="auto";if(item.range==="manual"&&Number.isFinite(item.min)&&Number.isFinite(item.max)&&item.min!<item.max!){result.range="manual";result.min=item.min;result.max=item.max}return Object.keys(result).length?result:undefined}
function restoreSeries(value:unknown,star=false):Record<string,DashboardSeriesAppearance>|undefined{if(!value||typeof value!=="object"||Array.isArray(value))return undefined;const entries=Object.entries(value).slice(0,64),result:Record<string,DashboardSeriesAppearance>={};for(const [key,raw] of entries){if(!key||key.length>260||!raw||typeof raw!=="object")continue;const item=raw as DashboardSeriesAppearance,next:DashboardSeriesAppearance={};if(typeof item.label==="string"&&item.label.trim())next.label=item.label.trim().slice(0,120);if(typeof item.unit==="string"&&item.unit.trim())next.unit=item.unit.trim().slice(0,64);if(typeof item.color==="string"&&/^#[0-9a-f]{6}$/i.test(item.color))next.color=item.color.toLowerCase();if(star&&["historical","manual","zero_to_one"].includes(String(item.normalization)))next.normalization=item.normalization;if(star&&item.normalization==="manual"&&Number.isFinite(item.min)&&Number.isFinite(item.max)&&item.min!<item.max!){next.min=item.min;next.max=item.max}if(Object.keys(next).length)result[key]=next}return Object.keys(result).length?result:undefined}
function finiteBetween(value:unknown,min:number,max:number):value is number{return typeof value==="number"&&Number.isFinite(value)&&value>=min&&value<=max}
function finite(value:unknown):value is number{return typeof value==="number"&&Number.isFinite(value)}
function fits(occupied:Set<string>,x:number,y:number,columns:number,rows:number){for(let row=y;row<y+rows;row++)for(let column=x;column<x+columns;column++)if(occupied.has(`${column}:${row}`))return false;return true}
function occupy(occupied:Set<string>,x:number,y:number,columns:number,rows:number){for(let row=y;row<y+rows;row++)for(let column=x;column<x+columns;column++)occupied.add(`${column}:${row}`)}
