import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Radio, TriangleAlert, WifiOff } from "lucide-react";
import { api } from "@/api";
import { TimeSeriesChart } from "@/components/time-series-chart";
import { Button } from "@/components/ui/button";
import { appendMetricUpdate, appendResourceUpdate, metricPoints, resourcePoints, seriesQuery, type TimeRange } from "@/lib/series";
import type { Job, MetricSeriesResponse, ResourceSeriesResponse, SeriesUpdate } from "@/types";

const ranges: TimeRange[] = ["1h", "6h", "24h", "7d", "all"];

export function MetricsPanel({ job }: { job: Job }) {
  const queryClient=useQueryClient();
  const [range, setRange] = useState<TimeRange>("all"), [end, setEnd] = useState(Date.now());
  const [live,setLive]=useState<"connecting"|"connected"|"reconnecting">("connecting");
  useEffect(()=>{if(job.finished_at)setEnd(Date.parse(job.finished_at))},[job.finished_at]);
  const query = useMemo(()=>seriesQuery(job,range,"auto",end),[job,range,end]);
  const metrics = useQuery({queryKey:["job-metrics",job.id,query],queryFn:()=>api.metrics(job.id,query),staleTime:30_000});
  const resources = useQuery({queryKey:["job-resources",job.id,query],queryFn:()=>api.resources(job.id,query),staleTime:30_000});
  const snapshotsReady=!!metrics.data&&!!resources.data;
  useEffect(()=>{
    if(!snapshotsReady||!job.attempt_id||job.finished_at)return;
    const after=Math.min(metrics.data!.cursor,resources.data!.cursor),source=api.openSeriesStream(job.id,job.attempt_id,after);
    setLive("connecting");
    source.onopen=()=>setLive("connected");source.onerror=()=>setLive("reconnecting");
    const receive=(event:MessageEvent)=>{const update=JSON.parse(event.data) as SeriesUpdate;queryClient.setQueryData<MetricSeriesResponse>(["job-metrics",job.id,query],current=>appendMetricUpdate(current,update));queryClient.setQueryData<ResourceSeriesResponse>(["job-resources",job.id,query],current=>appendResourceUpdate(current,update))};
    source.addEventListener("series",receive);return()=>source.close();
  },[snapshotsReady,query,job.id,job.attempt_id,job.finished_at,queryClient]);
  const resourceSeries = resources.data ? [
    {title:"CPU",points:resourcePoints(resources.data.points,p=>p.cpu_millis,1000),format:cores,color:"#3b82f6"},
    {title:"Memory",points:resourcePoints(resources.data.points,p=>p.memory_bytes,1073741824),format:gib,color:"#8b5cf6"},
    {title:"GPU utilization",points:resourcePoints(resources.data.points,p=>p.gpu_utilization_basis_points,100),format:percent,color:"#f59e0b"},
    {title:"GPU memory",points:resourcePoints(resources.data.points,p=>p.gpu_memory_bytes,1073741824),format:gib,color:"#ef4444"},
  ].filter(item=>item.points.length>0) : [];
  return <div className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-2"><div className="flex items-center gap-2"><div className="flex rounded-md border p-0.5">{ranges.map(item=><Button key={item} type="button" size="sm" variant={range===item?"secondary":"ghost"} className="h-7 px-2.5 text-xs" onClick={()=>{setRange(item);setEnd(Date.now())}}>{item === "all" ? "All" : item}</Button>)}</div>{!job.finished_at&&(live==="connected"?<span className="flex items-center gap-1.5 text-xs text-emerald-600"><Radio className="size-3.5 animate-pulse"/>Live</span>:<span className="flex items-center gap-1.5 text-xs text-amber-600"><WifiOff className="size-3.5"/>{live==="connecting"?"Connecting":"Reconnecting"}</span>)}</div><div className="flex gap-2"><Button asChild variant="outline" size="sm"><a href={api.metricsCSV(job.id,query)}><Download className="size-4"/>Metrics CSV</a></Button><Button asChild variant="outline" size="sm"><a href={api.resourcesCSV(job.id,query)}><Download className="size-4"/>Resources CSV</a></Button></div></div>
    {(metrics.data?.truncated||resources.data?.truncated)&&<p className="flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-300"><TriangleAlert className="size-4"/>The selected range exceeded the point limit. Choose a shorter range or a coarser resolution.</p>}
    <SeriesSection title="SDK metrics" description={metrics.data ? `${metrics.data.series.length} series · ${resolution(metrics.data.resolution_seconds)}` : "Loading metric series"} loading={metrics.isLoading} error={metrics.isError ? "Metric series could not be loaded." : undefined} empty={!metrics.isLoading&&metrics.data?.series.length===0}>{metrics.data?.series.map(series=><TimeSeriesChart key={series.name} title={series.name} points={metricPoints(series.points)} summary={{last:series.last,min:series.min,max:series.max}}/>)}</SeriesSection>
    <SeriesSection title="Resources" description={resources.data ? `${resources.data.points.length} samples · ${resolution(resources.data.resolution_seconds)}` : "Loading resource samples"} loading={resources.isLoading} error={resources.isError ? "Resource samples could not be loaded." : undefined} empty={!resources.isLoading&&resourceSeries.length===0}>{resourceSeries.map(series=><TimeSeriesChart key={series.title} {...series}/>)}</SeriesSection>
  </div>;
}

function SeriesSection({title,description,loading,error,empty,children}:{title:string;description:string;loading:boolean;error?:string;empty:boolean;children:ReactNode}){return <section><div className="mb-2"><h2 className="text-sm font-semibold">{title}</h2><p className="text-xs text-muted-foreground">{description}</p></div>{loading?<div className="h-52 animate-pulse rounded-md border bg-muted/30"/>:error?<div role="alert" className="rounded-md border border-destructive/30 p-8 text-center text-sm text-destructive">{error}</div>:empty?<div className="rounded-md border p-8 text-center text-sm text-muted-foreground">No data reported for this range.</div>:<div className="grid gap-3 xl:grid-cols-2">{children}</div>}</section>}
function resolution(seconds:number){return seconds===0?"raw":seconds<60?`${seconds}s`:`${seconds/60}m`}
function cores(value:number){return `${value.toFixed(2)} cores`}
function gib(value:number){return `${value.toFixed(2)} GiB`}
function percent(value:number){return `${value.toFixed(1)}%`}
