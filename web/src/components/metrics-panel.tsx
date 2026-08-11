import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Radio, TriangleAlert, WifiOff } from "lucide-react";
import { api } from "@/api";
import { TimeSeriesChart } from "@/components/time-series-chart";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { appendMetricUpdate, appendResourceUpdate, metricPoints, resourcePoints, seriesQuery } from "@/lib/series";
import type { Job, MetricSeriesResponse, ResourceSeriesResponse, SeriesUpdate } from "@/types";

export function MetricsPanel({ job }: { job: Job }) {
  const queryClient = useQueryClient();
  const [live, setLive] = useState<"connecting" | "connected" | "reconnecting">("connecting");
  const query = useMemo(() => seriesQuery(job, "all", "auto", Date.now()), [job.id, job.attempt_id, job.started_at, job.finished_at]);
  const metrics = useQuery({ queryKey: ["job-metrics", job.id, query], queryFn: () => api.metrics(job.id, query), staleTime: 30_000 });
  const resources = useQuery({ queryKey: ["job-resources", job.id, query], queryFn: () => api.resources(job.id, query), staleTime: 30_000 });
  const snapshotsReady = !!metrics.data && !!resources.data;
  useEffect(() => {
    if (!snapshotsReady || !job.attempt_id || job.finished_at) return;
    const after = Math.min(metrics.data!.cursor, resources.data!.cursor), source = api.openSeriesStream(job.id, job.attempt_id, after);
    setLive("connecting");
    source.onopen = () => setLive("connected"); source.onerror = () => setLive("reconnecting");
    const receive = (event: MessageEvent) => {
      const update = JSON.parse(event.data) as SeriesUpdate;
      queryClient.setQueryData<MetricSeriesResponse>(["job-metrics", job.id, query], current => appendMetricUpdate(current, update));
      queryClient.setQueryData<ResourceSeriesResponse>(["job-resources", job.id, query], current => appendResourceUpdate(current, update));
    };
    source.addEventListener("series", receive);
    return () => source.close();
  }, [snapshotsReady, query, job.id, job.attempt_id, job.finished_at, queryClient]);
  const resourceSeries = resources.data ? [
    { title: "CPU", points: resourcePoints(resources.data.points, point => point.cpu_millis, 1000), format: cores, color: "#3b82f6" },
    { title: "Memory", points: resourcePoints(resources.data.points, point => point.memory_bytes, 1073741824), format: gib, color: "#8b5cf6" },
    { title: "GPU utilization", points: resourcePoints(resources.data.points, point => point.gpu_utilization_basis_points, 100), format: percent, color: "#f59e0b" },
    { title: "GPU memory", points: resourcePoints(resources.data.points, point => point.gpu_memory_bytes, 1073741824), format: gib, color: "#ef4444" },
  ].filter(item => item.points.length > 0) : [];
  return <div className="flex h-[calc(100dvh-13rem)] min-h-[430px] flex-col gap-2 overflow-hidden">
    {(metrics.data?.truncated || resources.data?.truncated) && <p className="flex shrink-0 items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-300"><TriangleAlert className="size-4"/>The selected range exceeded the point limit. Choose a shorter range.</p>}
    <SeriesSection title="SDK metrics" downloadURL={api.metricsCSV(job.id, query)} live={!job.finished_at ? live : undefined} loading={metrics.isLoading} error={metrics.isError ? "Metric series could not be loaded." : undefined} empty={!metrics.isLoading && metrics.data?.series.length === 0}>{metrics.data?.series.map(series => <TimeSeriesChart key={series.name} title={series.name} points={metricPoints(series.points)} summary={{ last: series.last, min: series.min, max: series.max }}/>)}</SeriesSection>
    <SeriesSection title="Resources" downloadURL={api.resourcesCSV(job.id, query)} loading={resources.isLoading} error={resources.isError ? "Resource samples could not be loaded." : undefined} empty={!resources.isLoading && resourceSeries.length === 0}>{resourceSeries.map(series => <TimeSeriesChart key={series.title} {...series}/>)}</SeriesSection>
  </div>;
}

function SeriesSection({ title, downloadURL, live, loading, error, empty, children }: { title: string; downloadURL: string; live?: "connecting" | "connected" | "reconnecting"; loading: boolean; error?: string; empty: boolean; children: ReactNode }) {
  return <section className="flex min-h-0 flex-1 flex-col gap-1.5"><div className="flex h-6 shrink-0 items-center gap-1.5"><h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{title}</h2><Tooltip><TooltipTrigger asChild><Button asChild variant="ghost" size="icon" className="size-6"><a href={downloadURL} aria-label={`Download ${title} CSV`}><Download className="size-3"/></a></Button></TooltipTrigger><TooltipContent>Download CSV</TooltipContent></Tooltip>{live && (live === "connected" ? <span className="ml-1 flex items-center gap-1 text-[10px] text-emerald-600"><Radio className="size-3 animate-pulse"/>Live</span> : <span className="ml-1 flex items-center gap-1 text-[10px] text-amber-600"><WifiOff className="size-3"/>{live === "connecting" ? "Connecting" : "Reconnecting"}</span>)}</div>{loading ? <div className="min-h-0 flex-1 animate-pulse rounded-md border bg-muted/30"/> : error ? <div role="alert" className="grid min-h-0 flex-1 place-items-center rounded-md border border-destructive/30 text-sm text-destructive">{error}</div> : empty ? <div className="grid min-h-0 flex-1 place-items-center rounded-md border text-sm text-muted-foreground">No data reported for this range.</div> : <div className="grid min-h-0 flex-1 auto-rows-fr gap-2 md:grid-cols-2">{children}</div>}</section>;
}

function cores(value: number) { return `${value.toFixed(2)} cores`; }
function gib(value: number) { return `${value.toFixed(2)} GiB`; }
function percent(value: number) { return `${value.toFixed(1)}%`; }
