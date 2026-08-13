import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Activity, BarChart3, Download, LineChart, Plus, ScatterChart, Target, Trash2, Workflow } from "lucide-react";
import { ConfusionMatrixWidget } from "@/components/confusion-matrix-widget";
import { ObservationPlot } from "@/components/observation-plot";
import { ProgressWidget } from "@/components/progress-widget";
import { TimeSeriesChart, type ChartMarker } from "@/components/time-series-chart";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { createDashboardWidget, defaultDashboardWidgets, layoutDashboardWidgets, removeDashboardWidget, widgetCatalog, type DashboardSourceKind, type DashboardSources, type DashboardWidget, type DashboardWidgetType } from "@/lib/dashboard-widgets";
import type { SeriesPoint } from "@/lib/series";
import type { MatrixObservation, ProgressState } from "@/types";

export type NumericWidgetSource = { kind: "metric" | "resource"; name: string; title: string; points: SeriesPoint[]; color?: string; format?: (value:number)=>string; summary?: {last:number;min:number;max:number} };

export function ObservabilityDashboard({ attemptID, ready, numericSources, progress, matrices, markers, metricsDownloadURL, resourcesDownloadURL, live }: { attemptID: string; ready: boolean; numericSources: NumericWidgetSource[]; progress?: ProgressState; matrices: MatrixObservation[]; markers: ChartMarker[]; metricsDownloadURL: string; resourcesDownloadURL: string; live?: ReactNode }) {
  const [widgets,setWidgets]=useState<DashboardWidget[]>([]),initializedAttempt=useRef("");
  const sources=useMemo<DashboardSources>(()=>({metrics:numericSources.filter(item=>item.kind==="metric").map(item=>item.name),resources:numericSources.filter(item=>item.kind==="resource").map(item=>item.name),matrices:matrices.map(item=>item.name),progress:hasProgress(progress)}),[numericSources,matrices,progress]);
  useEffect(()=>{if(!ready||initializedAttempt.current===attemptID)return;initializedAttempt.current=attemptID;setWidgets(defaultDashboardWidgets(sources))},[attemptID,ready,sources]);
  const add=(type:DashboardWidgetType)=>setWidgets(current=>layoutDashboardWidgets([...current,createDashboardWidget(type,sources)]));
  const remove=(id:string)=>setWidgets(current=>removeDashboardWidget(current,id));
  const ordered=[...widgets].sort((a,b)=>a.position.y-b.position.y||a.position.x-b.position.x);
  return <section aria-label="Metrics dashboard">
    <div className="mb-2 flex h-8 items-center gap-1.5"><h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Dashboard</h2>{live}<DownloadButton href={metricsDownloadURL} label="Download SDK metrics CSV"/><DownloadButton href={resourcesDownloadURL} label="Download resources CSV"/><div className="ml-auto"><AddWidgetMenu onAdd={add}/></div></div>
    {!ready?<div className="min-h-[320px] animate-pulse rounded-md border bg-muted/30"/>:ordered.length===0?<EmptyDashboard onAdd={add}/>:<div className="grid gap-2 xl:grid-cols-2">{ordered.map(widget=><div key={widget.id} data-widget-id={widget.id} data-widget-type={widget.type} data-position={`${widget.position.x},${widget.position.y}`} data-size={`${widget.size.columns}x${widget.size.rows}`} className={cn("min-h-[320px]",widget.size.columns===2&&"xl:col-span-2")}><DashboardWidgetView widget={widget} numericSources={numericSources} progress={progress} matrices={matrices} markers={markers} onRemove={()=>remove(widget.id)}/></div>)}</div>}
  </section>;
}

function DashboardWidgetView({widget,numericSources,progress,matrices,markers,onRemove}:{widget:DashboardWidget;numericSources:NumericWidgetSource[];progress?:ProgressState;matrices:MatrixObservation[];markers:ChartMarker[];onRemove:()=>void}){
  const action=<RemoveWidgetButton onRemove={onRemove}/>;
  const source=widget.sources[0];
  if(widget.type==="progress"&&progress&&hasProgress(progress))return <ProgressWidget state={progress} actions={action}/>;
  if(widget.type==="confusion_matrix"&&source){const matrix=matrices.find(item=>item.name===source.name);if(matrix)return <ConfusionMatrixWidget matrix={matrix} actions={action}/>}
  const numeric=source&&numericSources.find(item=>item.kind===source.kind&&item.name===source.name);
  if(numeric&&widget.type==="lineplot")return <TimeSeriesChart title={numeric.title} points={numeric.points} color={numeric.color} format={numeric.format} summary={numeric.summary} markers={source.kind==="metric"?markers:[]} actions={action}/>;
  if(numeric&&(widget.type==="barplot"||widget.type==="scatterplot"))return <ObservationPlot type={widget.type} title={numeric.title} points={numeric.points} markers={source.kind==="metric"?markers:[]} actions={action}/>;
  return <UnavailableWidget type={widget.type} sourceKind={source?.kind} onRemove={onRemove}/>;
}

function AddWidgetMenu({onAdd}:{onAdd:(type:DashboardWidgetType)=>void}){return <DropdownMenu><DropdownMenuTrigger asChild><Button size="sm" className="h-7 gap-1.5"><Plus className="size-3.5"/>Add widget</Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-64">{widgetCatalog.map(item=>{const Icon=widgetIcon(item.type);return <DropdownMenuItem key={item.type} onSelect={()=>onAdd(item.type)} className="items-start"><Icon className="mt-0.5 size-4 shrink-0"/><span><span className="block font-medium">{item.label}</span><span className="block text-xs text-muted-foreground">{item.description}</span></span></DropdownMenuItem>})}</DropdownMenuContent></DropdownMenu>}
function EmptyDashboard({onAdd}:{onAdd:(type:DashboardWidgetType)=>void}){return <div className="grid min-h-[360px] place-items-center rounded-md border border-dashed"><div className="max-w-sm text-center"><Workflow className="mx-auto mb-3 size-8 text-muted-foreground"/><h3 className="text-sm font-medium">Build your metrics dashboard</h3><p className="mt-1 mb-4 text-sm text-muted-foreground">Add a visualization without changing or duplicating the job telemetry.</p><AddWidgetMenu onAdd={onAdd}/></div></div>}
function UnavailableWidget({type,sourceKind,onRemove}:{type:DashboardWidgetType;sourceKind?:DashboardSourceKind;onRemove:()=>void}){const entry=widgetCatalog.find(item=>item.type===type)!;return <section className="flex min-h-[320px] flex-col rounded-md border bg-card p-3"><header className="flex items-center"><h3 className="text-sm font-medium">{entry.label}</h3><div className="ml-auto"><RemoveWidgetButton onRemove={onRemove}/></div></header><div className="grid flex-1 place-items-center px-6 text-center text-sm text-muted-foreground">{sourceKind?"The selected source is no longer available for this attempt.":`No compatible ${type==="confusion_matrix"?"matrix":type==="progress"?"progress":"numeric series"} has been reported yet.`}</div></section>}
function RemoveWidgetButton({onRemove}:{onRemove:()=>void}){return <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="size-6" aria-label="Remove widget" onClick={onRemove}><Trash2 className="size-3"/></Button></TooltipTrigger><TooltipContent>Remove widget</TooltipContent></Tooltip>}
function DownloadButton({href,label}:{href:string;label:string}){return <Tooltip><TooltipTrigger asChild><Button asChild variant="ghost" size="icon" className="size-6"><a href={href} aria-label={label}><Download className="size-3"/></a></Button></TooltipTrigger><TooltipContent>Download CSV</TooltipContent></Tooltip>}
function hasProgress(progress?:ProgressState){return !!progress&&(progress.global_progress!=null||progress.simple!=null||progress.current!=null||(progress.milestones?.length??0)>0)}
function widgetIcon(type:DashboardWidgetType){return type==="lineplot"?LineChart:type==="barplot"?BarChart3:type==="scatterplot"?ScatterChart:type==="confusion_matrix"?Target:Activity}
