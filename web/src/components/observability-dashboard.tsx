import { useEffect, useMemo, useRef, useState, type DragEvent, type PointerEvent as ReactPointerEvent } from "react";
import { ChevronLeft, ChevronRight, Eraser, GripVertical, Maximize2, PaintBucket, Paintbrush, Plus, Settings2, Terminal, Trash2 } from "lucide-react";
import { createContext, useContext } from "react";
import { ConfusionMatrixWidget } from "@/components/confusion-matrix-widget";
import { DistributionWidget } from "@/components/distribution-widget";
import { DataGridWidget } from "@/components/data-grid-widget";
import { EvaluationCurveWidget } from "@/components/evaluation-curve-widget";
import { FeatureImportanceWidget } from "@/components/feature-importance-widget";
import { LiveLogs, type StreamName } from "@/components/live-logs";
import { HeatmapWidget } from "@/components/heatmap-widget";
import { ObservationPlot } from "@/components/observation-plot";
import { PartialDependenceWidget } from "@/components/partial-dependence-widget";
import { ProgressWidget } from "@/components/progress-widget";
import { ProjectionScatterWidget } from "@/components/projection-scatter-widget";
import { RegressionDiagnosticsWidget } from "@/components/regression-diagnostics-widget";
import { ScalarSummaryWidget } from "@/components/scalar-summary-widget";
import { ShapSummaryWidget } from "@/components/shap-summary-widget";
import { StarPlot } from "@/components/star-plot";
import { TabularChartWidget } from "@/components/tabular-chart-widget";
import { type ChartMarker } from "@/components/time-series-chart";
import { WidgetAppearanceEditor } from "@/components/widget-appearance-editor";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { compatibleSourceKinds, createDashboardWidget, defaultDashboardWidgets, layoutDashboardWidgets, moveDashboardWidget, removeDashboardWidget, resizeDashboardWidget, restoreDashboardWidgets, widgetCatalog, widgetCategoryLabels, type DashboardAppearance, type DashboardSourceKind, type DashboardSources, type DashboardWidget, type DashboardWidgetSize, type DashboardWidgetType, type WidgetCatalogCategory } from "@/lib/dashboard-widgets";
import { dashboardPalettes, effectiveColors, gradientFromColor, gradientFromColors, paletteByRef, quickColors, supportsGradient } from "@/lib/dashboard-palettes";
import { widgetIcons } from "@/lib/widget-icons";
import type { SeriesPoint } from "@/lib/series";
import type { DistributionObservation, MatrixObservation, ObservableSourceDescriptor, ProgressState } from "@/types";
export type NumericWidgetSource = {
    kind: "metric" | "resource";
    name: string;
    title: string;
    unit: string;
    points: SeriesPoint[];
    color?: string;
    format?: (value: number) => string;
    summary?: {
        last: number;
        min: number;
        max: number;
        avg: number;
    };
    phase?: string;
    tags?: string[];
    metadata?: Record<string, unknown>;
    declared?: boolean;
    observed?: boolean;
};
type WidgetSourceOption = {
    kind: DashboardSourceKind;
    name: string;
    title: string;
    subtype?: string;
    tags?: string[];
    unit?: string;
    phase?: string;
    declared?: boolean;
    observed?: boolean;
};
type Props = {
    jobID: string;
    attemptID: string;
    ready: boolean;
    numericSources: NumericWidgetSource[];
    observableSources?: ObservableSourceDescriptor[];
    progress?: ProgressState;
    matrices: MatrixObservation[];
    distributions?: DistributionObservation[];
    markers: ChartMarker[];
    initialWidgets?: DashboardWidget[] | null;
    dashboardAppearance?: DashboardAppearance;
    onDashboardAppearanceChange?: (appearance?: DashboardAppearance) => void;
    onWidgetsChange?: (widgets: DashboardWidget[]) => void;
    onWidgetsReady?: (widgets: DashboardWidget[]) => void;
    replacement?: {
        key: string;
        widgets: DashboardWidget[];
    };
    editMode?: boolean;
};
const DashboardAppearanceContext = createContext<DashboardAppearance | undefined>(undefined);
type AppearanceDragState={kind:"dashboard"|"palette"|"color"|"clear";value:string};
type PaintTarget={widgetID:string;seriesKey?:string};
export function ObservabilityDashboard({ jobID, attemptID, ready, numericSources, observableSources = [], progress, matrices, distributions = [], markers, initialWidgets = null, dashboardAppearance, onDashboardAppearanceChange, onWidgetsChange, onWidgetsReady, replacement, editMode = false }: Props) {
    const [widgets, setWidgets] = useState<DashboardWidget[]>([]), [preview, setPreview] = useState<DashboardWidget[] | null>(null), [dragging, setDragging] = useState(""), [panelOpen, setPanelOpen] = useState(true), [appearancePanelOpen, setAppearancePanelOpen] = useState(true), [selectedWidget, setSelectedWidget] = useState(""), [appearanceDrag,setAppearanceDrag]=useState<AppearanceDragState>(),[paintTarget,setPaintTarget]=useState<PaintTarget>(), initialized = useRef(""), appliedReplacement = useRef(""), hydrating = useRef(false), observedWidgets = useRef(widgets), palette = useRef<DashboardWidget | undefined>(undefined);
    const sourceOptions = useMemo<WidgetSourceOption[]>(() => {
        const options: WidgetSourceOption[] = [...numericSources];
        for (const descriptor of observableSources) {
            const kind = descriptor.type as DashboardSourceKind;
            if ((kind !== "matrix" && kind !== "progress" && kind !== "distribution" && kind !== "table") || options.some(item => item.kind === kind && item.name === descriptor.name))
                continue;
            options.push({ kind, name: descriptor.name, title: descriptor.name, subtype: kind === "matrix" ? matrixSubtype(descriptor.subtype, descriptor.tags) : descriptor.subtype, tags: descriptor.tags, unit: descriptor.unit, phase: descriptor.phase, declared: descriptor.declared, observed: descriptor.observed });
        }
        for (const matrix of matrices)
            if (!options.some(item => item.kind === "matrix" && item.name === matrix.name))
                options.push({ kind: "matrix", name: matrix.name, title: matrix.name, subtype: matrix.matrix_type, tags: matrix.tags, unit: matrix.unit, observed: true });
        for (const item of distributions)
            if (!options.some(option => option.kind === "distribution" && option.name === item.name))
                options.push({ kind: "distribution", name: item.name, title: item.name, unit: item.unit, observed: true });
        if (hasProgress(progress) && !options.some(item => item.kind === "progress"))
            options.push({ kind: "progress", name: "progress", title: "Progress", observed: true });
        return options;
    }, [numericSources, observableSources, matrices, distributions, progress]);
    const sources = useMemo<DashboardSources>(() => ({ metrics: numericSources.filter(item => item.kind === "metric").map(item => item.name), resources: numericSources.filter(item => item.kind === "resource").map(item => item.name), matrices: sourceOptions.filter(item => item.kind === "matrix").map(item => item.name), matrix_types: Object.fromEntries(sourceOptions.filter(item => item.kind === "matrix").map(item => [item.name, item.subtype ?? "confusion_matrix"])), distributions: sourceOptions.filter(item => item.kind === "distribution").map(item => item.name), tables: sourceOptions.filter(item => item.kind === "table").map(item => item.name), table_types: Object.fromEntries(sourceOptions.filter(item => item.kind === "table").map(item => [item.name, item.subtype ?? "table"])), progress: sourceOptions.some(item => item.kind === "progress"), logs: true }), [numericSources, sourceOptions]);
    useEffect(() => { if (!ready || initialized.current === attemptID)
        return; initialized.current = attemptID; hydrating.current = true; setWidgets(restoreDashboardWidgets(initialWidgets) ?? defaultDashboardWidgets(sources)); }, [attemptID, ready, sources, initialWidgets]);
    useEffect(() => { if (!replacement || replacement.key === appliedReplacement.current)
        return; appliedReplacement.current = replacement.key; hydrating.current = true; setWidgets(restoreDashboardWidgets(replacement.widgets) ?? []); }, [replacement]);
    useEffect(() => { if (observedWidgets.current === widgets)
        return; observedWidgets.current = widgets; if (hydrating.current) {
        hydrating.current = false;
        return;
    } if (initialized.current === attemptID && ready)
        onWidgetsChange?.(widgets); }, [widgets, attemptID, ready, onWidgetsChange]);
    useEffect(() => { if (initialized.current === attemptID && ready)
        onWidgetsReady?.(widgets); }, [widgets, attemptID, ready, onWidgetsReady]);
    useEffect(() => { if (!editMode) {
        setPreview(null);
        setDragging("");
    } }, [editMode]);
    const displayed = preview ?? widgets, ordered = [...displayed].sort((a, b) => a.position.y - b.position.y || a.position.x - b.position.x);
    const update = (next: DashboardWidget) => setWidgets(current => current.map(item => item.id === next.id ? next : item));
    const finishDrag = () => { setPreview(null); setDragging(""); setAppearanceDrag(undefined); setPaintTarget(undefined); palette.current = undefined; };
    const previewOver = (event: DragEvent, target?: string) => { event.preventDefault(); if (palette.current) {
        const added = layoutDashboardWidgets([...widgets, palette.current]);
        setPreview(target ? moveDashboardWidget(added, palette.current.id, target) : added);
        return;
    } if (dragging && target && dragging !== target)
        setPreview(moveDashboardWidget(widgets, dragging, target)); };
    const drop = (event: DragEvent) => { event.preventDefault(); const dashboardPalette = event.dataTransfer.getData("text/jobdock-dashboard-palette"); if (dashboardPalette) {
        onDashboardAppearanceChange?.({ schema_version: 1, palette: { id: dashboardPalette, version: 1 } });
        return;
    } if (preview)
        setWidgets(preview);
    else if (palette.current)
        setWidgets(layoutDashboardWidgets([...widgets, palette.current])); finishDrag(); };
    return <DashboardAppearanceContext.Provider value={dashboardAppearance}><section aria-label="Metrics dashboard" className="h-full min-h-0">
    {!ready ? <div className="h-full animate-pulse rounded-md border bg-muted/30"/> : <div className="flex h-full min-h-0 overflow-hidden rounded-md border bg-muted/10">
      {editMode && <WidgetPalette open={panelOpen} onToggle={() => setPanelOpen(value => !value)} onDragStart={(event, type) => { const item = createDashboardWidget(type, sources); palette.current = item; setDragging(item.id); event.dataTransfer.effectAllowed = "copy"; event.dataTransfer.setData("text/jobdock-widget-type", type); }} onDragEnd={finishDrag}/>}
      <div className={cn("relative min-w-0 flex-1 overflow-auto p-2", editMode && "bg-[linear-gradient(to_right,hsl(var(--border)/.22)_1px,transparent_1px),linear-gradient(to_bottom,hsl(var(--border)/.22)_1px,transparent_1px)] bg-[size:8.333%_8.333%]")} style={{background:paintTarget?.widgetID==="dashboard"&&appearanceDrag?appearancePreview(appearanceDrag):paletteByRef(dashboardAppearance?.palette)?.surface?.background}} onDragOver={event => { if (event.dataTransfer.types.includes("text/jobdock-dashboard-palette")) {
            event.preventDefault();
            setPaintTarget({widgetID:"dashboard"});
            return;
        } previewOver(event); }} onDragLeave={event=>{if(!event.currentTarget.contains(event.relatedTarget as Node))setPaintTarget(undefined)}} onDrop={event=>{drop(event);setPaintTarget(undefined)}}>
        {editMode && <div className={cn("pointer-events-none absolute inset-2 rounded-md border-2 border-dashed transition-colors", dragging ? "border-primary/50" : "border-transparent")} aria-hidden/>}
        {ordered.length === 0 ? <EmptyDashboard editing={editMode}/> : <div className="grid h-full min-h-0 grid-cols-12 grid-rows-12 auto-rows-fr gap-1">{ordered.map(widget => <div key={widget.id} data-widget-id={widget.id} data-widget-type={widget.type} data-position={`${widget.position.x},${widget.position.y}`} data-size={`${widget.size.columns}x${widget.size.rows}`} style={{ gridColumn: `span ${widget.size.columns}`, gridRow: `span ${widget.size.rows}` }} className={cn("relative min-h-0 min-w-0 transition-[opacity,transform]", dragging === widget.id && "opacity-45")} onDragOver={event => { event.stopPropagation(); if(event.dataTransfer.types.includes("text/jobdock-dashboard-palette")){event.preventDefault();setPaintTarget({widgetID:"dashboard"});}
        else if (event.dataTransfer.types.some(type => type.startsWith("text/jobdock-appearance"))){
            event.preventDefault();setPaintTarget({widgetID:widget.id});}
        else
            previewOver(event, widget.id); }} onDragLeave={event=>{if(!event.currentTarget.contains(event.relatedTarget as Node))setPaintTarget(undefined)}} onDrop={event => { event.stopPropagation(); const paletteID = event.dataTransfer.getData("text/jobdock-appearance-palette"), color = event.dataTransfer.getData("text/jobdock-appearance-color"),clear=event.dataTransfer.getData("text/jobdock-appearance-clear"); if (paletteID || color || clear) {
            update(applyAppearance(widget,clear?{kind:"clear",value:"clear"}:paletteID?{kind:"palette",value:paletteID}:{kind:"color",value:color}));setPaintTarget(undefined);
            return;
        } drop(event); }}>{editMode&&paintTarget?.widgetID===widget.id&&!paintTarget.seriesKey&&appearanceDrag&&<div data-paint-preview="widget" className="pointer-events-none absolute inset-0 z-30 grid place-items-center rounded-md border-2 border-current text-foreground shadow-inner" style={{background:appearancePreview(appearanceDrag)}}><span className="rounded-full bg-background/85 p-2 shadow"><PaintBucket className="size-5"/></span></div>}{editMode ? <EditWidgetShell widget={widget} selected={selectedWidget === widget.id} appearanceDrag={appearanceDrag} paintTarget={paintTarget} onPaintTarget={setPaintTarget} onSelect={() => setSelectedWidget(widget.id)} onDragStart={event => { if ((event.target as HTMLElement).closest("button") || (event.target as HTMLElement).closest("[data-series-target]")) {
            event.preventDefault();
            return;
        } event.dataTransfer.effectAllowed = "move"; event.dataTransfer.setData("text/jobdock-widget", widget.id); setDragging(widget.id); }} onDragEnd={finishDrag} onConfigure={update} sourceOptions={sourceOptions} onRemove={() => setWidgets(current => removeDashboardWidget(current, widget.id))} onResizePreview={size => setPreview(resizeDashboardWidget(widgets, widget.id, size))} onResizeCommit={size => { setWidgets(resizeDashboardWidget(widgets, widget.id, size)); setPreview(null); }}/> : <DashboardWidgetView jobID={jobID} attemptID={attemptID} widget={widget} numericSources={numericSources} sourceOptions={sourceOptions} progress={progress} matrices={matrices} distributions={distributions} markers={markers} onUpdate={update}/>}</div>)}</div>}
      </div>
      {editMode && <AppearancePalette open={appearancePanelOpen} onToggle={() => setAppearancePanelOpen(value => !value)} selected={widgets.find(item => item.id === selectedWidget)} onUpdate={update} dashboardAppearance={dashboardAppearance} onDashboardAppearanceChange={onDashboardAppearanceChange} onDragState={setAppearanceDrag} onDragEnd={finishDrag}/>}
    </div>}
  </section></DashboardAppearanceContext.Provider>;
}
function DashboardWidgetView({ jobID, attemptID, widget, numericSources, sourceOptions, progress, matrices, distributions, markers, onUpdate }: {
    jobID: string;
    attemptID: string;
    widget: DashboardWidget;
    numericSources: NumericWidgetSource[];
    sourceOptions: WidgetSourceOption[];
    progress?: ProgressState;
    matrices: MatrixObservation[];
    distributions: DistributionObservation[];
    markers: ChartMarker[];
    onUpdate: (widget: DashboardWidget) => void;
}) {
    const dashboardAppearance = useContext(DashboardAppearanceContext);
    useEffect(() => { const host = [...document.querySelectorAll<HTMLElement>("[data-widget-id]")].find(element => element.dataset.widgetId === widget.id); if (!host)
        return; const surface = paletteByRef(dashboardAppearance?.palette)?.surface, appearance = widget.appearance; host.classList.add("dashboard-widget-surface"); host.dataset.widgetPadding = appearance?.padding ?? "normal"; host.style.setProperty("--widget-bg", appearance?.background_color ?? surface?.card ?? ""); host.style.setProperty("--widget-border", appearance?.border_color ?? surface?.border ?? ""); host.style.setProperty("--widget-text", appearance?.text_color ?? surface?.text ?? ""); host.style.setProperty("--widget-accent", appearance?.accent_color ?? effectiveColors(dashboardAppearance, appearance)[0]); return () => { host.classList.remove("dashboard-widget-surface"); delete host.dataset.widgetPadding; for (const property of ["--widget-bg", "--widget-border", "--widget-text", "--widget-accent"])
        host.style.removeProperty(property); }; }, [dashboardAppearance, widget]);
    const source = widget.sources[0];
    const option = sourceOptions.find(item => source && item.kind === source.kind && item.name === source.name);
    const colors=effectiveColors(dashboardAppearance,widget.appearance),inheritedGradient=!widget.appearance?.gradient&&(widget.appearance?.palette||dashboardAppearance?.palette)&&supportsGradient(widget.type)?gradientFromColors(colors):undefined,effectiveAppearance=inheritedGradient?{schema_version:1 as const,...widget.appearance,gradient:inheritedGradient}:widget.appearance,resolvedWidget={...widget,appearance:{schema_version:1 as const,...effectiveAppearance,accent_color:effectiveAppearance?.accent_color??colors[0]}};
    if (option?.declared && option.observed === false)
        return <UnavailableWidget type={widget.type} sourceKind={source?.kind} waiting phase={option.phase}/>;
    if (widget.type === "logs") {
        const streams = widget.sources.filter(item => item.kind === "log" && (item.name === "stdout" || item.name === "stderr")).map(item => item.name as StreamName);
        const logAppearance={schema_version:1 as const,...effectiveAppearance,series:{...effectiveAppearance?.series,"log:stdout":{...effectiveAppearance?.series?.["log:stdout"],color:effectiveAppearance?.series?.["log:stdout"]?.color??colors[0]},"log:stderr":{...effectiveAppearance?.series?.["log:stderr"],color:effectiveAppearance?.series?.["log:stderr"]?.color??colors[1]}}};
        return <LiveLogs jobId={jobID} attemptId={attemptID} streams={streams.length ? streams : ["stdout"]} embedded appearance={logAppearance} actions={<ConfigureWidget widget={widget} sources={sourceOptions} onUpdate={onUpdate}/>}/>;
    }
    if (widget.type === "data_grid" && source?.kind === "table")
        return <DataGridWidget jobID={jobID} attemptID={attemptID} widget={widget} onUpdate={onUpdate}/>;
    if ((widget.type === "roc_curve" || widget.type === "precision_recall_curve" || widget.type === "calibration_curve") && source?.kind === "table")
        return <EvaluationCurveWidget jobID={jobID} attemptID={attemptID} widget={widget} colors={colors}/>;
    if ((widget.type === "prediction_vs_actual" || widget.type === "residual_plot") && source?.kind === "table")
        return <RegressionDiagnosticsWidget jobID={jobID} attemptID={attemptID} widget={widget} colors={colors}/>;
    if (widget.type === "feature_importance" && source?.kind === "table")
        return <FeatureImportanceWidget jobID={jobID} attemptID={attemptID} widget={widget} colors={colors} onUpdate={onUpdate}/>;
    if (widget.type === "shap_summary" && source?.kind === "table")
        return <ShapSummaryWidget jobID={jobID} attemptID={attemptID} widget={widget} colors={colors} onUpdate={onUpdate}/>;
    if (widget.type === "partial_dependence" && source?.kind === "table")
        return <PartialDependenceWidget jobID={jobID} attemptID={attemptID} widget={resolvedWidget}/>;
    if ((widget.type === "embedding_scatter" || widget.type === "cluster_scatter") && source?.kind === "table")
        return <ProjectionScatterWidget jobID={jobID} attemptID={attemptID} widget={widget} colors={colors} onUpdate={onUpdate}/>;
    if ((widget.type === "bubble_chart" || widget.type === "parallel_coordinates" || widget.type === "pie_chart" || widget.type === "donut_chart" || widget.type === "treemap" || widget.type === "waterfall") && source?.kind === "table")
        return <TabularChartWidget jobID={jobID} attemptID={attemptID} widget={widget} colors={colors} onUpdate={onUpdate}/>;
    if (widget.type === "progress" && progress && hasProgress(progress))
        return <ProgressWidget state={progress} appearance={effectiveAppearance}/>;
    if (widget.type === "confusion_matrix" && source) {
        const matrix = matrices.find(item => item.name === source.name);
        if (matrix)
            return <ConfusionMatrixWidget matrix={matrix} initialMode={effectiveAppearance?.matrix_mode} appearance={effectiveAppearance}/>;
    }
    if ((widget.type === "heatmap" || widget.type === "correlation_heatmap") && source) {
        const matrix = matrices.find(item => item.name === source.name);
        if (matrix && (widget.type === "heatmap" && matrix.matrix_type === "heatmap" || widget.type === "correlation_heatmap" && matrix.matrix_type === "correlation"))
            return <HeatmapWidget matrix={matrix} correlation={widget.type === "correlation_heatmap"} appearance={effectiveAppearance}/>;
    }
    if (widget.type === "histogram" || widget.type === "boxplot" || widget.type === "violin") {
        const names = new Set(widget.sources.filter(item => item.kind === "distribution").map(item => item.name)), items = distributions.filter(item => names.has(item.name));
        if (items.length)
            return <DistributionWidget type={widget.type} title={widget.title} items={items} appearance={effectiveAppearance} colors={colors}/>;
    }
    const paletteActive=!!(widget.appearance?.accent_color||widget.appearance?.palette||dashboardAppearance?.palette),numeric = widget.sources.flatMap((reference, index) => { const item = numericSources.find(candidate => candidate.kind === reference.kind && candidate.name === reference.name); return item ? [{ ...item, id: sourceKey(item), role: reference.role, color: effectiveAppearance?.series?.[sourceKey(reference)]?.color ?? (paletteActive?colors[index % colors.length]:item.color??colors[index % colors.length]) }] : []; }), widgetMarkers = widget.sources.some(item => item.kind === "metric") ? markers : [];
    const waiting = !!option?.declared && !hasWidgetData(widget, numeric, progress, matrices);
    if (waiting)
        return <UnavailableWidget type={widget.type} sourceKind={source?.kind} waiting phase={option?.phase}/>;
    if (numeric.length === 1 && (widget.type === "kpi" || widget.type === "gauge"))
        return <ScalarSummaryWidget widget={{...widget,appearance:effectiveAppearance}} source={numeric[0]}/>;
    if (numeric.length > 0 && widget.type === "anomaly_timeline")
        return <AnomalyTimeline widget={widget} series={numeric} markers={widgetMarkers}/>;
    if (numeric.length >= 3 && widget.type === "starplot")
        return <StarPlot title={widget.title} series={numeric} range={widget.time_range} appearance={widget.appearance}/>;
    if (numeric.length > 0 && (widget.type === "lineplot" || widget.type === "loss_curve" || widget.type === "learning_curve" || widget.type === "barplot" || widget.type === "area_chart" || widget.type === "stacked_bar")) {
        const semantic = widget.type === "loss_curve" || widget.type === "learning_curve", declaredAxis = semantic ? numeric.find(item => typeof item.metadata?.x_axis === "string")?.metadata?.x_axis : undefined, xAxis = declaredAxis === "time" ? "time" : semantic ? "step" : widget.x_axis, axisLabel = declaredAxis === "training_size" ? "Training size" : declaredAxis === "epoch" ? "Epoch" : declaredAxis === "step" ? "Step" : undefined, appearance = axisLabel ? { ...widget.appearance, schema_version: 1 as const, x_axis: { ...widget.appearance?.x_axis, label: axisLabel } } : widget.appearance;
        return <ObservationPlot type={widget.type === "loss_curve" || widget.type === "learning_curve" ? "lineplot" : widget.type} title={widget.title} series={numeric} xAxis={xAxis} markers={widgetMarkers} range={widget.time_range} appearance={appearance}/>;
    }
    if (numeric.length >= 2 && widget.type === "scatterplot") {
        const x = numeric.find(item => item.role === "x") ?? numeric[0], y = numeric.find(item => item.role === "y") ?? numeric[1];
        return <ObservationPlot type="scatterplot" title={widget.title} series={[x, y]} markers={widgetMarkers} range={widget.time_range} appearance={widget.appearance}/>;
    }
    return <UnavailableWidget type={widget.type} sourceKind={source?.kind}/>;
}
function AnomalyTimeline({ widget, series, markers }: {
    widget: DashboardWidget;
    series: Array<NumericWidgetSource & {
        id: string;
    }>;
    markers: ChartMarker[];
}) {
    const isThreshold = (item: NumericWidgetSource) => item.tags?.includes("metric:anomaly_threshold") || item.metadata?.anomaly_role === "threshold", isDetection = (item: NumericWidgetSource) => item.tags?.includes("metric:anomaly_detection") || item.metadata?.anomaly_role === "detection", scores = series.filter(item => !isThreshold(item) && !isDetection(item)), thresholds = series.filter(isThreshold), detections = series.filter(isDetection);
    const expanded = thresholds.map(item => { if (item.points.length !== 1 || scores.every(score => score.points.length === 0))
        return item; const scorePoints = scores.flatMap(score => score.points).sort((a, b) => a.timestamp - b.timestamp), first = scorePoints[0], last = scorePoints[scorePoints.length - 1], point = item.points[0]; return { ...item, points: [{ ...point, timestamp: first.timestamp, step: first.step }, { ...point, timestamp: last.timestamp, step: last.step }] }; }), detectionMarkers = detections.flatMap(item => item.points.filter(point => point.value !== 0).map((point, index) => ({ id: `anomaly:${item.id}:${point.timestamp}:${index}`, timestamp: point.timestamp, step: point.step, label: `Anomaly detected · ${item.title}` })));
    return <ObservationPlot type="lineplot" title={widget.title} series={[...scores, ...expanded]} xAxis={widget.x_axis} markers={[...markers, ...detectionMarkers]} range={widget.time_range} appearance={widget.appearance}/>;
}
function WidgetPalette({ open, onToggle, onDragStart, onDragEnd }: {
    open: boolean;
    onToggle: () => void;
    onDragStart: (event: DragEvent<HTMLElement>, type: DashboardWidgetType) => void;
    onDragEnd: () => void;
}) { const categories = (Object.keys(widgetCategoryLabels) as WidgetCatalogCategory[]).filter(category => widgetCatalog.some(item => item.category === category)); return <aside className={cn("relative z-10 shrink-0 border-r bg-background transition-[width]", open ? "w-64" : "w-11")} aria-label="Widget library"><Button variant="ghost" size="icon" className="absolute right-3 top-3 size-8" onClick={onToggle} aria-label={open ? "Collapse widget library" : "Expand widget library"}>{open ? <ChevronLeft className="size-4"/> : <ChevronRight className="size-4"/>}</Button>{open && <div className="h-full overflow-y-auto p-3"><p className="mb-3 flex h-8 items-center pr-10 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Widgets</p><div className="space-y-5">{categories.map(category => <section key={category} aria-labelledby={`widget-category-${category}`}><h3 id={`widget-category-${category}`} className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{widgetCategoryLabels[category]}</h3><div className="space-y-1.5">{widgetCatalog.filter(item => item.category === category).map(item => { const Icon = widgetIcon(item.type); return <div key={item.type} draggable onDragStart={event => onDragStart(event, item.type)} onDragEnd={onDragEnd} className="cursor-grab rounded-md border bg-card p-2.5 active:cursor-grabbing"><div className="flex items-center gap-2 text-sm font-medium"><Icon className="size-4"/>{item.label}</div><p className="mt-1 text-xs leading-snug text-muted-foreground">{item.description}</p></div>; })}</div></section>)}</div></div>}</aside>; }
function EditWidgetShell({ widget, selected, appearanceDrag, paintTarget, onPaintTarget, onSelect, onDragStart, onDragEnd, onConfigure, sourceOptions, onRemove, onResizePreview, onResizeCommit }: {
    widget: DashboardWidget;
    selected: boolean;
    appearanceDrag?:AppearanceDragState;
    paintTarget?:PaintTarget;
    onPaintTarget:(target?:PaintTarget)=>void;
    onSelect: () => void;
    onDragStart: (event: DragEvent<HTMLElement>) => void;
    onDragEnd: () => void;
    onConfigure: (widget: DashboardWidget) => void;
    sourceOptions: WidgetSourceOption[];
    onRemove: () => void;
    onResizePreview: (size: DashboardWidgetSize) => void;
    onResizeCommit: (size: DashboardWidgetSize) => void;
}) {
    const origin = useRef<{
        x: number;
        y: number;
        columnUnit: number;
        rowUnit: number;
        size: DashboardWidgetSize;
    } | undefined>(undefined), latest = useRef(widget.size), label = widgetCatalog.find(item => item.type === widget.type)?.label ?? widget.type;
    const finishResize = () => { if (!origin.current)
        return; onResizeCommit(latest.current); origin.current = undefined; document.body.style.userSelect = ""; window.removeEventListener("pointermove", moveResize, true); window.removeEventListener("pointerup", finishResize, true); window.removeEventListener("pointercancel", finishResize, true); };
    const moveResize = (event: globalThis.PointerEvent) => { const initial = origin.current; if (!initial)
        return; event.preventDefault(); const next = { columns: clamp(initial.size.columns + Math.round((event.clientX - initial.x) / initial.columnUnit), 1, 12), rows: clamp(initial.size.rows + Math.round((event.clientY - initial.y) / initial.rowUnit), 1, 12) }; if (next.columns === latest.current.columns && next.rows === latest.current.rows)
        return; latest.current = next; onResizePreview(next); };
    const start = (event: ReactPointerEvent<HTMLButtonElement>) => { event.preventDefault(); event.stopPropagation(); const tile = event.currentTarget.closest("section")!.getBoundingClientRect(); origin.current = { x: event.clientX, y: event.clientY, columnUnit: tile.width / widget.size.columns || 100, rowUnit: tile.height / widget.size.rows || 80, size: widget.size }; latest.current = widget.size; document.body.style.userSelect = "none"; window.addEventListener("pointermove", moveResize, true); window.addEventListener("pointerup", finishResize, true); window.addEventListener("pointercancel", finishResize, true); };
    const Icon = widgetIcon(widget.type);
    const setSeriesColor = (key: string, color?: string) => {const entry={...widget.appearance?.series?.[key]};if(color)entry.color=color;else delete entry.color;const series={...widget.appearance?.series};if(Object.keys(entry).length)series[key]=entry;else delete series[key];onConfigure({ ...widget, appearance: { schema_version: 1, ...widget.appearance, series } });};
    return <section draggable={!origin.current} onClick={onSelect} onDragStart={onDragStart} onDragEnd={onDragEnd} className={cn("relative flex h-full min-h-0 min-w-0 cursor-grab flex-col overflow-hidden rounded-md border-2 border-dashed bg-card/90 p-3 shadow-sm active:cursor-grabbing", selected ? "border-primary" : "border-primary/30")}><div className="flex min-w-0 items-start gap-3"><Icon className="mt-0.5 size-5 shrink-0 text-foreground/55"/><div className="min-w-0"><h3 className="truncate font-medium">{label}</h3></div><GripVertical className="ml-auto size-4 shrink-0 text-muted-foreground"/></div><div className="absolute inset-x-10 top-1/2 flex -translate-y-1/2 flex-wrap items-center justify-center gap-1.5">{widget.sources.length ? widget.sources.map(source => { const key = sourceKey(source), color = widget.appearance?.series?.[key]?.color,painting=paintTarget?.widgetID===widget.id&&paintTarget.seriesKey===key&&appearanceDrag; return <button type="button" data-series-target data-paint-preview={painting?"series":undefined} key={`${key}:${source.role ?? ""}`} className="flex max-w-[12rem] items-center gap-1.5 rounded-full border bg-background/90 px-2 py-1 text-[10px] shadow-sm transition-colors" style={{ borderColor: color,background:painting?appearancePreview(appearanceDrag):undefined }} onDragOver={event => { if (event.dataTransfer.types.includes("text/jobdock-appearance-color")||event.dataTransfer.types.includes("text/jobdock-appearance-palette")||event.dataTransfer.types.includes("text/jobdock-appearance-clear")) {
        event.preventDefault();
        event.stopPropagation();
        onPaintTarget({widgetID:widget.id,seriesKey:key});
    } }} onDragLeave={event=>{if(!event.currentTarget.contains(event.relatedTarget as Node))onPaintTarget(undefined)}} onDrop={event => { const direct = event.dataTransfer.getData("text/jobdock-appearance-color"),paletteID=event.dataTransfer.getData("text/jobdock-appearance-palette"),clear=event.dataTransfer.getData("text/jobdock-appearance-clear"),next=direct||paletteByRef(paletteID?{id:paletteID,version:1}:undefined)?.colors[0]; if (next||clear) {
        event.preventDefault();
        event.stopPropagation();
        setSeriesColor(key, clear?undefined:next);
        onPaintTarget(undefined);
        } }}><i className="size-2 rounded-full" style={{ background: color ?? "var(--muted-foreground)" }}/><span className="truncate">{source.role ? `${source.role}: ` : ""}{source.kind} / {source.name}</span></button>; }) : <span className="text-xs text-muted-foreground">No data source</span>}</div><div className="mt-auto flex min-w-0 items-center gap-1 pt-2"><span className="truncate text-[10px] uppercase tracking-wide text-muted-foreground">{widget.size.rows}×{widget.size.columns}</span><div className="absolute bottom-2 left-1/2 flex -translate-x-1/2 rounded-md border bg-background/90 shadow-sm"><ConfigureWidget widget={widget} sources={sourceOptions} onUpdate={onConfigure}/><AppearanceWidget widget={widget} sources={sourceOptions} onUpdate={onConfigure}/><Button variant="ghost" size="icon" className="size-7" aria-label="Remove widget" onClick={onRemove}><Trash2 className="size-3.5"/></Button></div></div><Button variant="ghost" size="icon" className="absolute bottom-0 right-0 size-8 cursor-nwse-resize touch-none" aria-label="Drag to resize widget" onPointerDown={start}><Maximize2 className="size-3.5"/></Button></section>;
}
function AppearancePalette({ open, onToggle, selected, onUpdate, dashboardAppearance, onDashboardAppearanceChange,onDragState,onDragEnd }: {
    open: boolean;
    onToggle: () => void;
    selected?: DashboardWidget;
    onUpdate: (widget: DashboardWidget) => void;
    dashboardAppearance?: DashboardAppearance;
    onDashboardAppearanceChange?: (appearance?: DashboardAppearance) => void;
    onDragState:(state?:AppearanceDragState)=>void;
    onDragEnd:()=>void;
}) {
    const [customColor,setCustomColor]=useState("#2563eb"),validCustomColor=/^#[0-9a-f]{6}$/i.test(customColor);
    useEffect(()=>{const explicit=selected?.type==="logs"?selected.appearance?.background_color:selected?.appearance?.accent_color;setCustomColor(explicit??"#2563eb")},[selected?.id,selected?.type,selected?.appearance?.accent_color,selected?.appearance?.background_color]);
    const drag = (event: DragEvent<HTMLElement>, kind: AppearanceDragState["kind"], value: string) => { event.dataTransfer.effectAllowed = "copy"; event.dataTransfer.setData(kind === "dashboard" ? "text/jobdock-dashboard-palette" : kind === "palette" ? "text/jobdock-appearance-palette" : kind==="clear"?"text/jobdock-appearance-clear":"text/jobdock-appearance-color", value);onDragState({kind,value}); setAppearanceDragImage(event, kind === "color" ? value : undefined,kind==="clear"); };
    const applyPalette = (id: string) => selected && onUpdate(applyAppearance(selected,{kind:"palette",value:id}));
    const applyColor=(color:string)=>selected&&onUpdate(applyAppearance(selected,{kind:"color",value:color})),clearColor=()=>selected&&onUpdate(applyAppearance(selected,{kind:"clear",value:"clear"}));
    return <aside className={cn("relative z-10 shrink-0 border-l bg-background transition-[width]", open ? "w-64" : "w-11")} aria-label="Appearance library"><Button variant="ghost" size="icon" className="absolute left-1.5 top-3 size-8" onClick={onToggle} aria-label={open ? "Collapse appearance library" : "Expand appearance library"}>{open ? <ChevronRight className="size-4"/> : <ChevronLeft className="size-4"/>}</Button>{open && <div className="h-full overflow-y-auto p-3"><p className="mb-3 flex h-8 items-center pl-10 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Appearance</p><PaletteSection title="Dashboard palettes">{dashboardPalettes.filter(item => item.scope === "dashboard").map(item => <PaletteButton key={item.id} item={item} active={dashboardAppearance?.palette?.id === item.id} onDragStart={event => drag(event, "dashboard", item.id)} onDragEnd={onDragEnd} onClick={() => onDashboardAppearanceChange?.({ schema_version: 1, palette: { id: item.id, version: 1 } })}/>)}</PaletteSection><PaletteSection title="Widget palettes">{dashboardPalettes.filter(item => item.scope === "widget").map(item => <PaletteButton key={item.id} item={item} active={selected?.appearance?.palette?.id === item.id} onDragStart={event => drag(event, "palette", item.id)} onDragEnd={onDragEnd} onClick={() => applyPalette(item.id)}/>)}</PaletteSection><PaletteSection title="Quick colors"><div className="grid grid-cols-5 gap-2"><button draggable aria-label="Remove explicit color" className="grid size-8 place-items-center rounded-md border shadow-sm" style={{background:"repeating-conic-gradient(#e2e8f0 0 25%,#ffffff 0 50%) 50%/10px 10px"}} onDragStart={event=>drag(event,"clear","clear")} onDragEnd={onDragEnd} onClick={clearColor}><Eraser className="size-4 text-slate-700"/></button>{quickColors.map(color => <button key={color} draggable aria-label={`Apply ${color}`} className="size-8 rounded-md border shadow-sm" style={{ background: color }} onDragStart={event => drag(event, "color", color)} onDragEnd={onDragEnd} onClick={() => applyColor(color)}/>)}</div><div className="mt-2 grid grid-cols-[2rem_1fr_2rem] items-center gap-1.5"><input aria-label="Custom color picker" type="color" className="size-8 cursor-pointer rounded border bg-transparent p-0.5" value={validCustomColor?customColor:"#2563eb"} onChange={event=>setCustomColor(event.target.value)}/><input aria-label="Custom color hex" className={cn("h-8 min-w-0 rounded-md border bg-background px-2 font-mono text-[10px] uppercase",!validCustomColor&&"border-destructive")} value={customColor} maxLength={7} onChange={event=>setCustomColor(event.target.value)}/><Button type="button" variant="outline" size="icon" className="size-8" aria-label="Apply custom color" disabled={!selected||!validCustomColor} onClick={()=>applyColor(customColor.toLowerCase())}><PaintBucket className="size-3.5"/></Button></div><button type="button" draggable={validCustomColor} disabled={!validCustomColor} aria-label={`Drag custom color ${validCustomColor?customColor:"invalid"}`} className="mt-1.5 flex h-7 w-full items-center justify-center rounded-md border text-[10px] font-medium shadow-sm disabled:opacity-50" style={{background:validCustomColor?customColor:undefined,color:validCustomColor?contrastColor(customColor):undefined}} onDragStart={event=>validCustomColor&&drag(event,"color",customColor.toLowerCase())} onDragEnd={onDragEnd}>Drag or apply custom color</button></PaletteSection>{dashboardAppearance && <Button variant="ghost" size="sm" className="mt-4 w-full" onClick={() => onDashboardAppearanceChange?.(undefined)}>Reset dashboard palette</Button>}</div>}</aside>;
}
function PaletteSection({ title, children }: {
    title: string;
    children: import("react").ReactNode;
}) { return <section className="mb-5"><h3 className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{title}</h3><div className="space-y-1.5">{children}</div></section>; }
function PaletteButton({ item, active, onDragStart, onDragEnd, onClick }: {
    item: (typeof dashboardPalettes)[number];
    active: boolean;
    onDragStart: (event: DragEvent<HTMLButtonElement>) => void;
    onDragEnd:()=>void;
    onClick: () => void;
}) { return <button type="button" draggable onDragStart={onDragStart} onDragEnd={onDragEnd} onClick={onClick} className={cn("flex w-full items-center gap-2 rounded-md border p-2 text-left text-xs", active && "border-primary ring-1 ring-primary/30")}><span className="flex overflow-hidden rounded">{item.colors.slice(0, 5).map(color => <i key={color} className="h-4 w-3" style={{ background: color }}/>)}</span><span className="truncate">{item.name}</span></button>; }
function setAppearanceDragImage(event: DragEvent<HTMLElement>, color?: string,clear=false) { const node = document.createElement("div"); node.style.cssText = `position:fixed;left:-1000px;top:-1000px;display:flex;align-items:center;gap:6px;padding:7px 9px;border-radius:8px;background:#fff;color:#111827;border:1px solid #cbd5e1;box-shadow:0 8px 24px #0003`; node.innerHTML = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="${clear?"#64748b":color ?? "#7c3aed"}" stroke-width="2"><path d="m19 11-8-8-8.6 8.6a2 2 0 0 0 0 2.8l5.2 5.2a2 2 0 0 0 2.8 0Z"/><path d="m5 2 5 5"/><path d="M2 13h15"/></svg>${clear?`<i style="width:18px;height:18px;border-radius:4px;background:repeating-conic-gradient(#cbd5e1 0 25%,#fff 0 50%) 50%/8px 8px"></i>`:color ? `<i style="width:18px;height:18px;border-radius:50%;background:${color}"></i>` : `<i style="width:28px;height:12px;border-radius:4px;background:linear-gradient(90deg,#2563eb,#7c3aed,#16a34a,#f59e0b,#dc2626)"></i>`}`; document.body.appendChild(node); event.dataTransfer.setDragImage(node, 12, 12); setTimeout(() => node.remove()); }
function applyAppearance(widget:DashboardWidget,drag:AppearanceDragState){if(drag.kind==="clear"){const rest={schema_version:1 as const,...widget.appearance};delete rest.palette;delete rest.accent_color;delete rest.gradient;if(widget.type==="logs")delete rest.background_color;return{...widget,appearance:rest}}const palette=drag.kind==="palette"?paletteByRef({id:drag.value,version:1}):undefined,gradient=supportsGradient(widget.type)?drag.kind==="color"?gradientFromColor(drag.value):palette?gradientFromColors(palette.colors):undefined:undefined;return{...widget,appearance:{schema_version:1 as const,...widget.appearance,...(palette?{palette:{id:palette.id,version:1 as const}}:drag.kind==="color"?(widget.type==="logs"?{background_color:drag.value}:{accent_color:drag.value}):{}),...(gradient?{gradient}:{})}}}
function appearancePreview(drag:AppearanceDragState){if(drag.kind==="clear")return"repeating-conic-gradient(rgb(226 232 240 / .72) 0 25%,rgb(255 255 255 / .72) 0 50%) 50%/16px 16px";if(drag.kind==="color")return`color-mix(in srgb, ${drag.value} 42%, transparent)`;const palette=paletteByRef({id:drag.value,version:1});return palette?`linear-gradient(135deg,${gradientFromColors(palette.colors).map(stop=>`${stop.color} ${stop.offset*100}%`).join(",")})`:"transparent"}
function contrastColor(color:string){const value=parseInt(color.slice(1),16),red=value>>16,green=value>>8&255,blue=value&255;return red*.299+green*.587+blue*.114>150?"#111827":"#ffffff"}
function ConfigureWidget({ widget, sources, onUpdate }: {
    widget: DashboardWidget;
    sources: WidgetSourceOption[];
    onUpdate: (widget: DashboardWidget) => void;
}) {
    const [open, setOpen] = useState(false), [draft, setDraft] = useState(widget);
    useEffect(() => { if (open)
        setDraft(widget); }, [open, widget]);
    const numericSources = sources.filter((item): item is NumericWidgetSource => (item.kind === "metric" || item.kind === "resource") && "points" in item), compatibleSources = widgetSourceOptions(widget.type, sources), stepCompatible = draft.sources.length > 0 && draft.sources.every(reference => numericSources.find(item => sourceKey(item) === sourceKey(reference))?.points.some(point => point.step != null)), stacked = widget.type === "stacked_bar" || widget.type === "area_chart" && draft.appearance?.stack_mode === "stacked", selectedNumeric = draft.sources.flatMap(reference => { const source = numericSources.find(item => sourceKey(item) === sourceKey(reference)); return source ? [source] : []; }), mixedStackUnits = stacked && new Set(selectedNumeric.map(source => source.unit || "unitless")).size > 1, isMulti = multiSourceWidget(widget.type);
    const setSingle = (value: string) => { const item = compatibleSources.find(candidate => sourceKey(candidate) === value); if (item)
        setDraft(current => ({ ...current, sources: [{ kind: item.kind, name: item.name }] })); }, setScatter = (role: "x" | "y", value: string) => { const item = numericSources.find(source => sourceKey(source) === value); if (item)
        setDraft(current => ({ ...current, sources: [...current.sources.filter(source => source.role !== role), { kind: item.kind, name: item.name, role }] })); }, x = draft.sources.find(item => item.role === "x"), y = draft.sources.find(item => item.role === "y"), minimum = widget.type === "starplot" ? 3 : 1, invalid = scalarConfigurationInvalid(draft) || mixedStackUnits || (widget.type === "scatterplot" ? draft.sources.filter(item => item.role === "x" || item.role === "y").length < 2 : draft.sources.length < minimum);
    return <Dialog open={open} onOpenChange={setOpen}><Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="size-7" aria-label="Configure widget data" onClick={() => setOpen(true)}><Settings2 className="size-3.5"/></Button></TooltipTrigger><TooltipContent>Configure data</TooltipContent></Tooltip><DialogContent className="max-h-[90dvh] max-w-2xl overflow-y-auto"><DialogHeader><DialogTitle>Data · {widgetCatalog.find(item => item.type === widget.type)?.label}</DialogTitle><DialogDescription>Choose only the observations represented by this widget.</DialogDescription></DialogHeader>
    {widget.type === "logs" ? <fieldset className="space-y-2"><legend className="text-sm font-medium">Streams</legend>{(["stdout", "stderr"] as const).map(stream => <label key={stream} className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={draft.sources.some(item => item.kind === "log" && item.name === stream)} onChange={event => setDraft(current => ({ ...current, sources: event.target.checked ? [...current.sources.filter(item => item.kind !== "log" || item.name !== stream), { kind: "log", name: stream }] : current.sources.filter(item => item.kind !== "log" || item.name !== stream) }))}/>{stream}</label>)}</fieldset>
            : widget.type === "kpi" || widget.type === "gauge" ? <ScalarConfiguration draft={draft} sources={numericSources} onChange={setDraft}/>
                : widget.type === "scatterplot" ? <div className="grid gap-4 sm:grid-cols-2"><SourceSelect label="X series" value={x ? sourceKey(x) : ""} sources={numericSources} onChange={value => setScatter("x", value)}/><SourceSelect label="Y series" value={y ? sourceKey(y) : ""} sources={numericSources} onChange={value => setScatter("y", value)}/></div>
                    : isMulti ? <MultiSourceEditor draft={draft} sources={compatibleSources} minimum={minimum} maximum={widget.type === "starplot" ? 16 : widget.type === "roc_curve" || widget.type === "precision_recall_curve" || widget.type === "calibration_curve" ? 8 : 64} onChange={setDraft}/>
                        : <SourceSelect label="Source" value={draft.sources[0] ? sourceKey(draft.sources[0]) : ""} sources={compatibleSources} onChange={setSingle}/>}
    {temporalWidget(widget.type) && <><div><label className="mb-1.5 block text-sm font-medium">Horizontal axis</label><Select value={draft.x_axis ?? "time"} onValueChange={value => setDraft(current => ({ ...current, x_axis: value as "time" | "step" }))}><SelectTrigger aria-label="Horizontal axis"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="time">Captured time</SelectItem><SelectItem value="step" disabled={!stepCompatible}>Step</SelectItem></SelectContent></Select></div><TimeRangeConfiguration draft={draft} onChange={setDraft}/></>}
    {mixedStackUnits && <p role="alert" className="text-xs text-destructive">Stacked charts require every selected series to use the same unit.</p>}<DialogFooter><Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button><Button disabled={invalid} onClick={() => { onUpdate(draft); setOpen(false); }}>Apply</Button></DialogFooter></DialogContent></Dialog>;
}
function AppearanceWidget({ widget, sources, onUpdate }: {
    widget: DashboardWidget;
    sources: WidgetSourceOption[];
    onUpdate: (widget: DashboardWidget) => void;
}) {
    const [open, setOpen] = useState(false), [draft, setDraft] = useState(widget);
    useEffect(() => { if (open)
        setDraft(widget); }, [open, widget]);
    const appearanceSources = draft.sources.map(reference => { const source = sources.find(item => sourceKey(item) === sourceKey(reference)); return { key: sourceKey(reference), label: source?.title ?? reference.name, unit: source?.unit, color: draft.appearance?.series?.[sourceKey(reference)]?.color }; });
    return <Dialog open={open} onOpenChange={setOpen}><Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="size-7" aria-label="Configure widget appearance" onClick={() => setOpen(true)}><Paintbrush className="size-3.5"/></Button></TooltipTrigger><TooltipContent>Configure appearance</TooltipContent></Tooltip><DialogContent className="max-h-[90dvh] max-w-4xl overflow-y-auto"><DialogHeader><DialogTitle>Appearance · {widgetCatalog.find(item => item.type === widget.type)?.label}</DialogTitle></DialogHeader><WidgetAppearanceEditor draft={draft} sources={appearanceSources} onChange={setDraft}/><DialogFooter className="sm:justify-between"><Button variant="ghost" onClick={() => setDraft({ ...widget, title: undefined, appearance: undefined })}>Reset to defaults</Button><div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button><Button onClick={() => { onUpdate({ ...draft, title: draft.title?.trim() || undefined }); setOpen(false); }}>Apply</Button></div></DialogFooter></DialogContent></Dialog>;
}
function MultiSourceEditor({ draft, sources, minimum, maximum, onChange }: {
    draft: DashboardWidget;
    sources: WidgetSourceOption[];
    minimum: number;
    maximum: number;
    onChange: React.Dispatch<React.SetStateAction<DashboardWidget>>;
}) {
    const selected = new Set(draft.sources.map(sourceKey)), available = sources.filter(source => !selected.has(sourceKey(source))), [candidate, setCandidate] = useState("");
    useEffect(() => { if (candidate && !available.some(source => sourceKey(source) === candidate))
        setCandidate(""); }, [candidate, available]);
    const add = () => { const source = available.find(item => sourceKey(item) === candidate); if (source)
        onChange(current => ({ ...current, sources: [...current.sources, { kind: source.kind, name: source.name }] })); };
    return <fieldset className="space-y-2"><legend className="text-sm font-medium">Series</legend><div className="space-y-1 rounded-md border p-2">{draft.sources.map(reference => { const source = sources.find(item => sourceKey(item) === sourceKey(reference)); return <div key={sourceKey(reference)} className="flex h-9 items-center gap-2 rounded px-2 hover:bg-muted"><span className="min-w-0 flex-1 truncate text-sm">{source ? sourceOptionLabel(source) : reference.name}</span>{source?.unit && <span className="text-xs text-muted-foreground">{source.unit}</span>}<Button type="button" variant="ghost" size="icon" className="size-7" aria-label={`Remove ${source?.title ?? reference.name}`} disabled={draft.sources.length <= minimum} onClick={() => onChange(current => ({ ...current, sources: current.sources.filter(item => sourceKey(item) !== sourceKey(reference)) }))}><Trash2 className="size-3.5"/></Button></div>; })}{draft.sources.length === 0 && <p className="px-2 py-3 text-sm text-muted-foreground">No series selected.</p>}</div><div className="flex gap-2"><Select value={candidate} onValueChange={setCandidate}><SelectTrigger aria-label="Series to add" className="min-w-0 flex-1"><SelectValue placeholder="Add another series"/></SelectTrigger><SelectContent>{available.map(source => <SelectItem key={sourceKey(source)} value={sourceKey(source)}>{sourceOptionLabel(source)}{source.declared && !source.observed ? " · Waiting" : ""}</SelectItem>)}</SelectContent></Select><Button type="button" size="icon" variant="outline" aria-label="Add selected series" disabled={!candidate || draft.sources.length >= maximum} onClick={add}><Plus className="size-4"/></Button></div><p className="text-[11px] text-muted-foreground">{draft.sources.length} of {maximum} sources · minimum {minimum}</p></fieldset>;
}
function widgetSourceOptions(type: DashboardWidgetType, sources: WidgetSourceOption[]) {
    if (type === "confusion_matrix" || type === "heatmap" || type === "correlation_heatmap")
        return matrixSourcesForWidget(type, sources);
    const kinds = compatibleSourceKinds(type), expected = tableSubtype(type);
    return sources.filter(source => kinds.includes(source.kind) && (source.kind !== "table" || !expected || source.subtype === expected));
}
function tableSubtype(type: DashboardWidgetType) { return type === "roc_curve" ? "roc" : type === "precision_recall_curve" ? "precision_recall" : type === "calibration_curve" ? "calibration" : type === "prediction_vs_actual" || type === "residual_plot" ? "regression_diagnostics" : type === "feature_importance" ? "feature_importance" : type === "shap_summary" ? "shap_attribution" : type === "partial_dependence" ? "partial_dependence" : type === "embedding_scatter" || type === "cluster_scatter" ? "projection" : type === "bubble_chart" ? "bubble" : type === "parallel_coordinates" ? "multivariate" : type === "pie_chart" || type === "donut_chart" ? "categorical" : type === "treemap" ? "hierarchy" : type === "waterfall" ? "waterfall" : undefined; }
function multiSourceWidget(type: DashboardWidgetType) { return ["lineplot", "loss_curve", "learning_curve", "anomaly_timeline", "barplot", "area_chart", "stacked_bar", "starplot", "histogram", "boxplot", "violin", "roc_curve", "precision_recall_curve", "calibration_curve"].includes(type); }
function temporalWidget(type: DashboardWidgetType) { return ["lineplot", "loss_curve", "learning_curve", "anomaly_timeline", "barplot", "area_chart", "stacked_bar", "scatterplot", "starplot"].includes(type); }
function ScalarConfiguration({ draft, sources, onChange }: {
    draft: DashboardWidget;
    sources: NumericWidgetSource[];
    onChange: React.Dispatch<React.SetStateAction<DashboardWidget>>;
}) { const source = draft.sources[0], setNumber = (key: "target_value" | "warning_value" | "critical_value" | "domain_min" | "domain_max", value: string) => onChange(current => ({ ...current, [key]: value === "" ? undefined : Number(value) })); return <div className="space-y-4"><SourceSelect label="Value source" value={source ? sourceKey(source) : ""} sources={sources} onChange={value => { const item = sources.find(candidate => sourceKey(candidate) === value); if (item)
    onChange(current => ({ ...current, sources: [{ kind: item.kind, name: item.name }] })); }}/><div className="grid gap-4 sm:grid-cols-3"><label className="grid gap-1.5 text-sm font-medium">Aggregation<Select value={draft.scalar_aggregation ?? "last"} onValueChange={value => onChange(current => ({ ...current, scalar_aggregation: value as DashboardWidget["scalar_aggregation"] }))}><SelectTrigger aria-label="Scalar aggregation"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="last">Latest value</SelectItem><SelectItem value="min">Minimum</SelectItem><SelectItem value="max">Maximum</SelectItem><SelectItem value="avg">Average</SelectItem></SelectContent></Select></label>{draft.type === "gauge" && <label className="grid gap-1.5 text-sm font-medium">Presentation<Select value={draft.gauge_style ?? "gauge"} onValueChange={value => onChange(current => ({ ...current, gauge_style: value as "gauge" | "bullet" }))}><SelectTrigger aria-label="Gauge presentation"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="gauge">Gauge</SelectItem><SelectItem value="bullet">Bullet</SelectItem></SelectContent></Select></label>}<label className="grid gap-1.5 text-sm font-medium">Threshold direction<Select value={draft.threshold_direction ?? "higher_is_worse"} onValueChange={value => onChange(current => ({ ...current, threshold_direction: value as DashboardWidget["threshold_direction"] }))}><SelectTrigger aria-label="Threshold direction"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="higher_is_worse">Higher is worse</SelectItem><SelectItem value="lower_is_worse">Lower is worse</SelectItem></SelectContent></Select></label></div>{draft.type === "kpi" && <label className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm"><input type="checkbox" checked={draft.show_delta ?? false} onChange={event => onChange(current => ({ ...current, show_delta: event.target.checked }))}/>Show delta from previous observation</label>}<fieldset className="grid gap-3 rounded-md border p-3 sm:grid-cols-3"><legend className="px-1 text-sm font-medium">Targets and thresholds</legend><ScalarNumber label="Target" value={draft.target_value} onChange={value => setNumber("target_value", value)}/><ScalarNumber label="Warning" value={draft.warning_value} onChange={value => setNumber("warning_value", value)}/><ScalarNumber label="Critical" value={draft.critical_value} onChange={value => setNumber("critical_value", value)}/></fieldset><fieldset className="grid gap-3 rounded-md border p-3 sm:grid-cols-2"><legend className="px-1 text-sm font-medium">Domain</legend><ScalarNumber label="Minimum" value={draft.domain_min} onChange={value => setNumber("domain_min", value)}/><ScalarNumber label="Maximum" value={draft.domain_max} onChange={value => setNumber("domain_max", value)}/></fieldset>{scalarConfigurationInvalid(draft) && <p role="alert" className="text-xs text-destructive">Domain and thresholds must use finite, increasing values in the selected direction.</p>}</div>; }
function ScalarNumber({ label, value, onChange }: {
    label: string;
    value?: number;
    onChange: (value: string) => void;
}) { return <label className="grid gap-1 text-xs text-muted-foreground">{label}<input aria-label={label} type="number" step="any" className="h-9 rounded-md border bg-background px-3 text-sm text-foreground" value={value ?? ""} onChange={event => onChange(event.target.value)}/></label>; }
function scalarConfigurationInvalid(widget: DashboardWidget) { if (widget.type !== "kpi" && widget.type !== "gauge")
    return false; const values = [widget.target_value, widget.warning_value, widget.critical_value, widget.domain_min, widget.domain_max].filter(value => value != null); if (values.some(value => !Number.isFinite(value)))
    return true; if (widget.domain_min != null && widget.domain_max != null && widget.domain_min >= widget.domain_max)
    return true; if (widget.warning_value != null && widget.critical_value != null)
    return widget.threshold_direction === "lower_is_worse" ? widget.warning_value <= widget.critical_value : widget.warning_value >= widget.critical_value; return false; }
function TimeRangeConfiguration({ draft, onChange }: {
    draft: DashboardWidget;
    onChange: React.Dispatch<React.SetStateAction<DashboardWidget>>;
}) { return <div><label className="mb-1.5 block text-sm font-medium">Time range</label><Select value={draft.time_range ?? "all"} onValueChange={value => onChange(current => ({ ...current, time_range: value as DashboardWidget["time_range"] }))}><SelectTrigger aria-label="Time range"><SelectValue /></SelectTrigger><SelectContent>{(["1h", "6h", "24h", "7d", "all"] as const).map(value => <SelectItem key={value} value={value}>{value === "all" ? "All" : value}</SelectItem>)}</SelectContent></Select></div>; }
function SourceSelect({ label, value, sources, onChange }: {
    label: string;
    value: string;
    sources: WidgetSourceOption[];
    onChange: (value: string) => void;
}) { return <div><label className="mb-1.5 block text-sm font-medium">{label}</label><Select value={value} onValueChange={onChange}><SelectTrigger aria-label={label}><SelectValue placeholder="Select a source"/></SelectTrigger><SelectContent>{sources.map(source => <SelectItem key={sourceKey(source)} value={sourceKey(source)}>{sourceOptionLabel(source)}{source.declared && !source.observed ? " · Waiting" : ""}</SelectItem>)}</SelectContent></Select></div>; }
function EmptyDashboard({ editing }: {
    editing: boolean;
}) { return <div className="grid h-full min-h-0 place-items-center rounded-md border border-dashed"><div className="max-w-sm text-center"><Terminal className="mx-auto mb-3 size-8 text-muted-foreground"/><h3 className="text-sm font-medium">No dashboard widgets</h3><p className="mt-1 text-sm text-muted-foreground">{editing ? "Drag a widget from the library into this area." : "Enter edit mode to add observability widgets."}</p></div></div>; }
function UnavailableWidget({ type, sourceKind, waiting = false, phase }: {
    type: DashboardWidgetType;
    sourceKind?: DashboardSourceKind;
    waiting?: boolean;
    phase?: string;
}) { return <section className="flex h-full min-h-0 flex-col rounded-md border bg-card p-3"><h3 className="text-sm font-medium">{widgetCatalog.find(item => item.type === type)?.label}</h3><div className="grid flex-1 place-items-center px-6 text-center"><div><p className="text-sm text-muted-foreground">{waiting ? "Waiting for data" : sourceKind ? "The selected source is no longer available for this attempt." : "No compatible data has been reported yet."}</p>{waiting && phase && <p className="mt-1 text-xs text-muted-foreground">Phase: {phase}</p>}</div></div></section>; }
function sourceDescription(widget: DashboardWidget) { if (!widget.sources.length)
    return "No data source"; return widget.sources.map(source => `${source.role ? `${source.role}: ` : ""}${source.kind} / ${source.name}`).join(" · "); }
function sourceKey(source: {
    kind: string;
    name: string;
}) { return `${source.kind}:${source.name}`; }
function hasProgress(progress?: ProgressState) { return !!progress && (progress.global_progress != null || progress.simple != null || progress.current != null || (progress.milestones?.length ?? 0) > 0); }
function hasWidgetData(widget: DashboardWidget, numeric: NumericWidgetSource[], progress: ProgressState | undefined, matrices: MatrixObservation[]) { if (widget.type === "progress")
    return hasProgress(progress); if (widget.type === "confusion_matrix" || widget.type === "heatmap" || widget.type === "correlation_heatmap")
    return matrices.some(item => widget.sources.some(source => source.kind === "matrix" && source.name === item.name)); return numeric.some(item => item.points.length > 0); }
function sourceOptionLabel(source: WidgetSourceOption) { return source.phase ? `${source.phase} · ${source.title}` : source.title; }
function widgetIcon(type: DashboardWidgetType) { return widgetIcons[type]; }
function widgetIconElement(type: DashboardWidgetType) { const Icon = widgetIcon(type); return <Icon className="size-4"/>; }
function clamp(value: number, min: number, max: number) { return Math.max(min, Math.min(max, value)); }
function matrixSubtype(subtype?: string, tags?: string[]) { if (subtype)
    return subtype; if (tags?.includes("matrix:correlation"))
    return "correlation"; if (tags?.includes("matrix:heatmap"))
    return "heatmap"; return "confusion_matrix"; }
function matrixSourcesForWidget(type: DashboardWidgetType, sources: WidgetSourceOption[]) { return sources.filter(item => item.kind === "matrix" && (type === "heatmap" ? item.subtype === "heatmap" : type === "correlation_heatmap" ? item.subtype === "correlation" : item.subtype === "confusion_matrix" || !item.subtype)); }
