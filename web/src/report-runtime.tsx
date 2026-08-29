import React, { useMemo, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Star, TriangleAlert, X } from "lucide-react";
import { createRoot } from "react-dom/client";
import { ObservabilityDashboard, type NumericWidgetSource } from "@/components/observability-dashboard";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { DashboardAppearance, DashboardWidget } from "@/lib/dashboard-widgets";
import { configureOfflineDashboardSnapshot, type OfflineLogFragment } from "@/lib/offline-dashboard-data";
import { metricPoints, numericSummary, resourcePoints } from "@/lib/series";
import type { Checkpoint, DistributionObservation, MatrixObservation, MetricSeries, ObservableSourceDescriptor, ProgressState, ResourcePoint, TablePage } from "@/types";
import "./styles.css";
import "./report.css";

type DashboardReportWarning={dashboard_id?:string;widget_id?:string;source?:string;code:string;message:string};
type DashboardReportDashboard={id:string;name:string;is_default:boolean;schema_version:number;updated_at:string;config:{widgets:DashboardWidget[];appearance?:DashboardAppearance}};
type DashboardReportManifest={
  schema_version:number;
  jobdock_version:string;
  generated_at:string;
  theme:"light"|"dark";
  job:{id:string;name:string;status:string;created_at:string;started_at?:string;finished_at?:string};
  attempt:{id:string;attempt_number:number;status:string;created_at:string;started_at?:string;finished_at?:string};
  dashboards:DashboardReportDashboard[];
  sources:{metrics:MetricSeries[];resources:ResourcePoint[];matrices:Record<string,MatrixObservation>;distributions:Record<string,DistributionObservation[]>;tables:Record<string,TablePage>;logs:Record<string,string>;log_fragments:OfflineLogFragment[];progress?:ProgressState;checkpoints:Checkpoint[]};
  warnings:DashboardReportWarning[];
};

function decodeManifest():DashboardReportManifest{
  const encoded=document.getElementById("jobdock-report-data")?.textContent??"";
  const bytes=Uint8Array.from(atob(encoded),character=>character.charCodeAt(0));
  return JSON.parse(new TextDecoder().decode(bytes)) as DashboardReportManifest;
}

const manifest=decodeManifest();
document.documentElement.classList.toggle("dark",manifest.theme==="dark");
configureOfflineDashboardSnapshot({attemptID:manifest.attempt.id,tables:manifest.sources.tables,logFragments:manifest.sources.log_fragments??legacyLogFragments(manifest.sources.logs)});
const queryClient=new QueryClient({defaultOptions:{queries:{retry:false,staleTime:Infinity,gcTime:Infinity},mutations:{retry:false}}});

function App(){
  const[activeID,setActiveID]=useState(manifest.dashboards[0]?.id??""),[notesOpen,setNotesOpen]=useState(false);
  const dashboard=manifest.dashboards.find(item=>item.id===activeID)??manifest.dashboards[0];
  const numericSources=useMemo(buildNumericSources,[]),observableSources=useMemo(buildObservableSources,[]),markers=useMemo(()=>manifest.sources.checkpoints.flatMap(checkpoint=>checkpoint.timestamp?[{id:checkpoint.id,timestamp:Date.parse(checkpoint.timestamp),label:checkpoint.label||checkpoint.id.slice(0,8),step:checkpoint.step}]:[]),[]);
  const distributions=useMemo(()=>Object.values(manifest.sources.distributions).flat(),[]);
  if(!dashboard)return <div className="grid h-dvh place-items-center text-sm text-muted-foreground">No dashboards were included in this report.</div>;
  const warnings=manifest.warnings.filter(item=>!item.dashboard_id||item.dashboard_id===dashboard.id);
  return <TooltipProvider delayDuration={250}>
    <main className="relative flex h-dvh min-h-0 min-w-0 overflow-hidden bg-background text-foreground" aria-label={`${manifest.job.name} metrics report`}>
      <ReadOnlyDashboardRail dashboards={manifest.dashboards} activeID={dashboard.id} onSelect={setActiveID}/>
      <section className="min-h-0 min-w-0 flex-1 pl-2">
        <ObservabilityDashboard key={dashboard.id} jobID={manifest.job.id} attemptID={manifest.attempt.id} ready numericSources={numericSources} observableSources={observableSources} progress={manifest.sources.progress} matrices={Object.values(manifest.sources.matrices)} distributions={distributions} markers={markers} initialWidgets={dashboard.config.widgets} dashboardAppearance={dashboard.config.appearance} sourceWarnings={warnings.map(item=>({widgetID:item.widget_id,source:item.source,message:item.message}))}/>
      </section>
      {manifest.warnings.length>0&&<div className="absolute right-2 top-2 z-[80]">
        <Tooltip><TooltipTrigger asChild><button type="button" aria-label={`${manifest.warnings.length} export notes`} onClick={()=>setNotesOpen(value=>!value)} className="grid size-8 place-items-center rounded-md border border-amber-500/30 bg-background/90 text-amber-600 shadow-sm backdrop-blur hover:bg-amber-500/10 dark:text-amber-300"><TriangleAlert className="size-4"/></button></TooltipTrigger><TooltipContent side="left">Export notes</TooltipContent></Tooltip>
        {notesOpen&&<aside role="dialog" aria-label="Export notes" className="absolute right-0 top-10 w-80 max-w-[calc(100vw-4rem)] rounded-md border bg-popover p-3 text-popover-foreground shadow-xl"><div className="mb-2 flex items-center justify-between"><strong className="text-xs">Export notes</strong><button type="button" aria-label="Close export notes" onClick={()=>setNotesOpen(false)} className="rounded p-1 hover:bg-accent"><X className="size-3.5"/></button></div><ul className="max-h-64 space-y-2 overflow-auto text-xs text-muted-foreground">{manifest.warnings.map((warning,index)=><li key={`${warning.code}-${index}`}><span className="font-medium text-foreground">{warning.code.replaceAll("_"," ")}</span><br/>{warning.message}</li>)}</ul></aside>}
      </div>}
      <p data-report-trace className="sr-only">JobDock {manifest.jobdock_version}. Offline report schema {manifest.schema_version}. Job {manifest.job.id}. Attempt {manifest.attempt.id}. Generated {manifest.generated_at}.</p>
    </main>
  </TooltipProvider>;
}

function ReadOnlyDashboardRail({dashboards,activeID,onSelect}:{dashboards:DashboardReportDashboard[];activeID:string;onSelect:(id:string)=>void}){
  const navigate=(event:React.KeyboardEvent,index:number)=>{if(event.key!=="ArrowDown"&&event.key!=="ArrowUp")return;event.preventDefault();const next=(index+(event.key==="ArrowDown"?1:-1)+dashboards.length)%dashboards.length;onSelect(dashboards[next].id);requestAnimationFrame(()=>document.querySelector<HTMLButtonElement>(`[data-report-dashboard-id="${dashboards[next].id}"]`)?.focus())};
  return <aside className="flex h-full min-h-0 w-11 shrink-0 flex-col border-r pr-2 pt-2"><div role="tablist" aria-label="Job dashboards" aria-orientation="vertical" className="min-h-0 flex-1 space-y-1 overflow-y-auto overflow-x-hidden">{dashboards.map((item,index)=><Tooltip key={item.id}><TooltipTrigger asChild><button type="button" role="tab" data-report-dashboard-id={item.id} aria-selected={item.id===activeID} aria-label={item.name} tabIndex={item.id===activeID?0:-1} onClick={()=>onSelect(item.id)} onKeyDown={event=>navigate(event,index)} className={`group/rail relative flex h-9 w-full items-center justify-center overflow-visible rounded-md px-0 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring ${item.id===activeID?"bg-accent font-medium text-foreground":"text-muted-foreground hover:bg-accent/60 hover:text-foreground"}`}>{item.id===activeID&&<span className="absolute inset-y-1 left-0 w-0.5 rounded-full bg-primary"/>}{item.is_default&&<span aria-label={`${item.name} is default`} className="pointer-events-none absolute left-0.5 top-0.5 z-10 grid size-3 place-items-center rounded-full bg-background"><Star aria-hidden="true" className="size-2.5 fill-amber-500 text-amber-500"/></span>}<span className="grid size-6 shrink-0 place-items-center rounded text-[10px] font-semibold uppercase">{dashboardInitials(item.name)}</span></button></TooltipTrigger><TooltipContent side="right">{item.name}{item.is_default?" · default":""}</TooltipContent></Tooltip>)}</div></aside>;
}

function buildNumericSources():NumericWidgetSource[]{
  const metrics:NumericWidgetSource[]=manifest.sources.metrics.map(series=>({kind:"metric",name:series.name,title:series.name,unit:series.unit||"unitless",points:metricPoints(series.points),summary:{last:series.last,min:series.min,max:series.max,avg:series.avg??numericSummary(metricPoints(series.points))?.avg??series.last},tags:series.tags,metadata:series.metadata as Record<string,unknown>|undefined,phase:typeof series.metadata?.phase==="string"?series.metadata.phase:undefined,declared:true,observed:series.points.length>0}));
  const resources=([
    {kind:"resource",name:"cpu",title:"CPU",unit:"cores",points:resourcePoints(manifest.sources.resources,point=>point.cpu_millis,1000),format:cores,color:"#3b82f6"},
    {kind:"resource",name:"memory",title:"Memory",unit:"GiB",points:resourcePoints(manifest.sources.resources,point=>point.memory_bytes,1073741824),format:gib,color:"#8b5cf6"},
    {kind:"resource",name:"gpu-utilization",title:"GPU utilization",unit:"%",points:resourcePoints(manifest.sources.resources,point=>point.gpu_utilization_basis_points,100),format:percent,color:"#f59e0b"},
    {kind:"resource",name:"gpu-memory",title:"GPU memory",unit:"GiB",points:resourcePoints(manifest.sources.resources,point=>point.gpu_memory_bytes,1073741824),format:gib,color:"#ef4444"},
  ] satisfies NumericWidgetSource[]).filter(item=>item.points.length>0).map(item=>({...item,summary:numericSummary(item.points)??undefined}));
  return[...metrics,...resources];
}

function buildObservableSources():ObservableSourceDescriptor[]{
  const items:ObservableSourceDescriptor[]=[];
  for(const series of manifest.sources.metrics)items.push({name:series.name,type:"metric",unit:series.unit,tags:series.tags,metadata:series.metadata,phase:typeof series.metadata?.phase==="string"?series.metadata.phase:undefined,declared:true,observed:series.points.length>0});
  for(const matrix of Object.values(manifest.sources.matrices))items.push({name:matrix.name,type:"matrix",subtype:matrix.matrix_type,tags:matrix.tags,unit:matrix.unit,metadata:matrix.metadata,declared:true,observed:true});
  for(const [name,groups] of Object.entries(manifest.sources.distributions)){const first=groups[0];items.push({name,type:"distribution",unit:first?.unit,tags:first?.tags,metadata:first?.metadata,declared:true,observed:groups.length>0})}
  for(const [name,page] of Object.entries(manifest.sources.tables))items.push({name,type:"table",subtype:page.subtype,tags:page.tags,metadata:page.metadata,declared:true,observed:true});
  if(manifest.sources.progress)items.push({name:"progress",type:"progress",declared:true,observed:true});
  for(const stream of ["stdout","stderr"] as const)items.push({name:stream,type:"log",declared:true,observed:(manifest.sources.log_fragments??[]).some(item=>item.stream===stream)||!!manifest.sources.logs[stream]});
  return items;
}

function legacyLogFragments(logs:Record<string,string>):OfflineLogFragment[]{return(["stdout","stderr"] as const).flatMap(stream=>logs[stream]?[{stream,text:logs[stream]}]:[])}
function dashboardInitials(name:string){const words=name.trim().split(/\s+/).filter(Boolean);return(words.length>1?words.slice(0,2).map(word=>word[0]).join(""):name.slice(0,2)).toUpperCase()||"DB"}
function cores(value:number){return`${value.toFixed(2)} cores`}
function gib(value:number){return`${value.toFixed(2)} GiB`}
function percent(value:number){return`${value.toFixed(1)}%`}

createRoot(document.getElementById("root")!).render(<QueryClientProvider client={queryClient}><App/></QueryClientProvider>);
