import type { Job, MetricPoint, MetricSeriesResponse, ResourcePoint, ResourceSeriesResponse, SeriesUpdate } from "@/types";

export type SeriesPoint = { timestamp: number; value: number; step?: number; sampleCount?: number };
export type TimeRange = "1h" | "6h" | "24h" | "7d" | "all";

export function seriesQuery(job: Job, range: TimeRange, resolution = "auto", end = Date.now()) {
  const to = new Date(job.finished_at ?? end);
  const start = new Date(job.started_at ?? job.created_at);
  const duration = range === "all" ? Number.POSITIVE_INFINITY : { "1h": 3600e3, "6h": 21600e3, "24h": 86400e3, "7d": 604800e3 }[range];
  const from = new Date(Math.max(start.getTime(), to.getTime() - duration));
  const params = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), resolution, limit: "2000" });
  if (job.attempt_id) params.set("attempt_id", job.attempt_id);
  return params.toString();
}

export function metricPoints(points: MetricPoint[]): SeriesPoint[] {
  return points.map(point => ({ timestamp: Date.parse(point.captured_at), value: point.value, step: point.step, sampleCount: point.sample_count }));
}

export function resourcePoints(points: ResourcePoint[], selector: (point: ResourcePoint) => number | undefined, scale = 1): SeriesPoint[] {
  return points.flatMap(point => {
    const value = selector(point);
    return value == null ? [] : [{ timestamp: Date.parse(point.captured_at), value: value / scale, sampleCount: point.sample_count }];
  });
}

export function numericSummary(points: SeriesPoint[]) {
  if (points.length === 0) return null;
  let min = points[0].value, max = points[0].value;
  for (const point of points) { min = Math.min(min, point.value); max = Math.max(max, point.value); }
  return { min, max, last: points[points.length - 1].value };
}

export function zoomDomain(domain: [number, number], factor: number, anchor = .5): [number, number] {
  const [start, end] = domain, width = Math.max(1, end - start), next = Math.max(1000, width * factor);
  const center = start + width * Math.min(1, Math.max(0, anchor));
  return [center - next * anchor, center + next * (1 - anchor)];
}

const livePointLimit = 2000;

export function appendMetricUpdate(current: MetricSeriesResponse | undefined, update: SeriesUpdate) {
  if (!current || update.kind !== "metrics" || update.attempt_id !== current.attempt_id || update.cursor <= current.cursor) return current;
  const byName = new Map(current.series.map(series => [series.name, {...series, points:[...series.points]}]));
  for (const sample of update.metrics ?? []) {
    const point: MetricPoint = {cursor:sample.cursor,captured_at:sample.captured_at,step:sample.step,value:sample.value,sample_count:1};
    const series = byName.get(sample.name);
    if (series) {
	  if (sample.unit !== undefined) series.unit = sample.unit;
	  if (sample.metadata !== undefined) series.metadata = sample.metadata;
      series.points.push(point);
      if (series.points.length > livePointLimit) series.points.splice(0, series.points.length-livePointLimit);
      series.last=sample.value;series.min=Math.min(series.min,sample.value);series.max=Math.max(series.max,sample.value);series.sample_count+=1;
    } else byName.set(sample.name,{name:sample.name,unit:sample.unit,metadata:sample.metadata,points:[point],last:sample.value,min:sample.value,max:sample.value,sample_count:1});
  }
  return {...current,cursor:update.cursor,to:new Date(Math.max(Date.parse(current.to),Date.parse(update.captured_at))).toISOString(),series:[...byName.values()]};
}

export function appendResourceUpdate(current: ResourceSeriesResponse | undefined, update: SeriesUpdate) {
  if (!current || update.kind !== "resources" || update.attempt_id !== current.attempt_id || update.cursor <= current.cursor || !update.resource) return current;
  const points=[...current.points,update.resource];
  if(points.length>livePointLimit)points.splice(0,points.length-livePointLimit);
  return {...current,cursor:update.cursor,to:new Date(Math.max(Date.parse(current.to),Date.parse(update.captured_at))).toISOString(),points};
}
