import type { ReactNode } from "react";
import type { DashboardWidgetType } from "@/lib/dashboard-widgets";
import { widgetCatalog } from "@/lib/dashboard-widgets";

const scatterTypes = new Set<DashboardWidgetType>(["prediction_vs_actual", "residual_plot", "embedding_scatter", "cluster_scatter", "bubble_chart"]);
const curveTypes = new Set<DashboardWidgetType>(["roc_curve", "precision_recall_curve", "calibration_curve", "partial_dependence"]);

export function TemplateDummyWidget({ type }: { type: DashboardWidgetType }) {
  const label = widgetCatalog.find(item => item.type === type)?.label ?? humanize(type);
  return <div role="img" aria-label={`${label} sample preview`} data-dummy-widget-type={type} className="relative h-full min-h-0 overflow-hidden bg-background p-2">
    <span className="absolute left-2 top-1 z-10 text-[9px] font-medium text-muted-foreground/80">{label}</span>
    <div className="h-full min-h-0 pt-3">{dummyGraphic(type)}</div>
  </div>;
}

function dummyGraphic(type: DashboardWidgetType) {
  if (type === "data_grid") return <DataGridDummy />;
  if (type === "pie_chart" || type === "donut_chart") return <CompositionDummy donut={type === "donut_chart"} />;
  if (type === "treemap") return <TreemapDummy />;
  if (type === "waterfall") return <WaterfallDummy />;
  if (type === "feature_importance") return <FeatureImportanceDummy />;
  if (type === "shap_summary") return <ShapDummy />;
  if (type === "parallel_coordinates") return <ParallelDummy />;
  if (scatterTypes.has(type)) return <ScatterDummy />;
  if (curveTypes.has(type)) return <CurveDummy calibration={type === "calibration_curve"} />;
  return <CurveDummy />;
}

function PlotFrame({ children }: { children: ReactNode }) {
  return <svg viewBox="0 0 320 180" className="block size-full" aria-hidden="true">
    {[45, 85, 125].map(y => <line key={y} x1="38" x2="306" y1={y} y2={y} className="stroke-border/70" strokeDasharray="3 5" />)}
    <line x1="38" x2="306" y1="148" y2="148" className="stroke-muted-foreground" />
    <line x1="38" x2="38" y1="22" y2="148" className="stroke-muted-foreground" />
    {children}
    <text x="172" y="169" textAnchor="middle" className="fill-muted-foreground text-[9px]">X</text>
    <text x="12" y="85" textAnchor="middle" transform="rotate(-90 12 85)" className="fill-muted-foreground text-[9px]">Y</text>
  </svg>;
}

function CurveDummy({ calibration = false }: { calibration?: boolean }) {
  return <PlotFrame>
    {calibration && <line x1="38" y1="148" x2="306" y2="22" className="stroke-muted-foreground/60" strokeDasharray="5 5" />}
    <path d="M38 142 C76 136 89 116 121 110 S174 74 208 69 S259 37 306 28" fill="none" className="stroke-primary" strokeWidth="3" />
    {[{ x: 38, y: 142 }, { x: 121, y: 110 }, { x: 208, y: 69 }, { x: 306, y: 28 }].map(point => <circle key={point.x} cx={point.x} cy={point.y} r="3" className="fill-primary" />)}
  </PlotFrame>;
}

function ScatterDummy() {
  const points = [[64, 130, 4], [91, 118, 5], [116, 124, 3], [142, 93, 5], [170, 100, 4], [196, 72, 6], [226, 79, 4], [258, 49, 6], [286, 35, 4]];
  return <PlotFrame>{points.map(([x, y, r]) => <circle key={`${x}-${y}`} cx={x} cy={y} r={r} className="fill-primary/70 stroke-primary" />)}</PlotFrame>;
}

function FeatureImportanceDummy() {
  return <svg viewBox="0 0 320 180" className="block size-full" aria-hidden="true">
    {[{ y: 28, w: 205 }, { y: 58, w: 166 }, { y: 88, w: 132 }, { y: 118, w: 94 }].map((bar, index) => <g key={bar.y}><text x="48" y={bar.y + 12} textAnchor="end" className="fill-muted-foreground text-[9px]">F{index + 1}</text><rect x="56" y={bar.y} width={bar.w} height="16" rx="3" className="fill-primary/70" /></g>)}
    <line x1="56" x2="56" y1="18" y2="148" className="stroke-muted-foreground" />
  </svg>;
}

function ShapDummy() {
  const rows = [[-42, -17, 9, 31, 54], [-50, -24, 4, 19, 39], [-31, -10, 14, 28, 47], [-35, -4, 6, 21, 34]];
  return <svg viewBox="0 0 320 180" className="block size-full" aria-hidden="true">
    <line x1="174" x2="174" y1="18" y2="153" className="stroke-muted-foreground/70" />
    {rows.map((row, rowIndex) => <g key={rowIndex}><text x="52" y={38 + rowIndex * 31} textAnchor="end" className="fill-muted-foreground text-[9px]">F{rowIndex + 1}</text>{row.map((offset, index) => <circle key={offset} cx={174 + offset} cy={35 + rowIndex * 31 + (index % 2 ? 4 : -3)} r="5" className={index < 2 ? "fill-sky-500/75" : "fill-rose-500/75"} />)}</g>)}
  </svg>;
}

function ParallelDummy() {
  const axes = [54, 124, 194, 264];
  const lines = [[132, 52, 106, 39], [86, 121, 46, 93], [39, 75, 132, 64], [113, 30, 77, 125]];
  return <svg viewBox="0 0 320 180" className="block size-full" aria-hidden="true">{axes.map((x, index) => <g key={x}><line x1={x} x2={x} y1="24" y2="146" className="stroke-muted-foreground/70" /><text x={x} y="164" textAnchor="middle" className="fill-muted-foreground text-[8px]">F{index + 1}</text></g>)}{lines.map((values, index) => <polyline key={index} points={values.map((value, axis) => `${axes[axis]},${value + 16}`).join(" ")} fill="none" className="stroke-primary/55" strokeWidth="2" />)}</svg>;
}

function CompositionDummy({ donut }: { donut: boolean }) {
  return <div className="flex size-full items-center justify-center gap-5"><div className="relative aspect-square h-[78%] max-w-[55%] rounded-full" style={{ background: "conic-gradient(var(--primary) 0 42%, color-mix(in oklab, var(--primary) 65%, transparent) 42% 73%, color-mix(in oklab, var(--primary) 35%, transparent) 73%)" }}>{donut && <span className="absolute inset-[27%] rounded-full bg-background" />}</div><div className="space-y-2">{["A", "B", "C"].map((item, index) => <div key={item} className="flex items-center gap-1.5 text-[9px] text-muted-foreground"><span className="size-2 rounded-sm bg-primary" style={{ opacity: 1 - index * .25 }} />{item}</div>)}</div></div>;
}

function TreemapDummy() {
  return <div className="grid size-full grid-cols-5 grid-rows-3 gap-1 p-2"><span className="col-span-3 row-span-3 rounded bg-primary/75" /><span className="col-span-2 row-span-2 rounded bg-primary/50" /><span className="rounded bg-primary/35" /><span className="rounded bg-primary/25" /></div>;
}

function WaterfallDummy() {
  const bars = [{ x: 44, y: 82, h: 66 }, { x: 91, y: 59, h: 23 }, { x: 138, y: 59, h: 37 }, { x: 185, y: 72, h: 24 }, { x: 232, y: 45, h: 27 }, { x: 279, y: 45, h: 103 }];
  return <PlotFrame>{bars.map((bar, index) => <rect key={bar.x} x={bar.x} y={bar.y} width="27" height={bar.h} rx="2" className={index === 0 || index === bars.length - 1 ? "fill-primary/80" : index % 2 ? "fill-emerald-500/70" : "fill-rose-500/70"} />)}</PlotFrame>;
}

function DataGridDummy() {
  return <div className="grid h-full grid-cols-4 grid-rows-5 overflow-hidden rounded border border-border/70">{Array.from({ length: 20 }, (_, index) => <span key={index} className={`border-b border-r border-border/60 ${index < 4 ? "bg-muted/80" : index % 3 === 0 ? "bg-muted/30" : "bg-background"}`} />)}</div>;
}

function humanize(value: string) { return value.replaceAll("_", " ").replace(/\b\w/g, match => match.toUpperCase()); }
