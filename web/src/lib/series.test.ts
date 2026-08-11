import { describe, expect, it } from "vitest";
import { appendMetricUpdate, appendResourceUpdate, numericSummary, resourcePoints, seriesQuery, zoomDomain } from "./series";

describe("series helpers", () => {
  it("builds an attempt-scoped bounded query", () => {
    const query = new URLSearchParams(seriesQuery({id:"job",owner_id:"owner",attempt_id:"attempt",spec:{} as never,status:"SUCCEEDED",desired_status:"SUCCEEDED",observed_status:"SUCCEEDED",created_at:"2026-08-12T10:00:00Z",finished_at:"2026-08-12T12:00:00Z",version:1}, "1h"));
    expect(query.get("attempt_id")).toBe("attempt");
    expect(query.get("limit")).toBe("2000");
    expect(query.get("from")).toBe("2026-08-12T11:00:00.000Z");
  });
  it("does not invent GPU points for CPU-only samples", () => {
    expect(resourcePoints([{attempt_id:"a",captured_at:"2026-08-12T10:00:00Z",resolution_seconds:5,sample_count:1,cpu_millis:10,memory_bytes:20}], point => point.gpu_memory_bytes)).toEqual([]);
  });
  it("summarizes and zooms a numeric domain", () => {
    expect(numericSummary([{timestamp:1,value:3},{timestamp:2,value:1},{timestamp:3,value:5}])).toEqual({min:1,max:5,last:5});
    expect(zoomDomain([0,10_000], .5)).toEqual([2_500,7_500]);
  });
  it("appends only new attempt-scoped live deltas", () => {
    const metrics={attempt_id:"attempt",cursor:4,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:0,truncated:false,series:[{name:"loss",points:[],last:2,min:2,max:2,sample_count:1}]};
    const metricUpdate={cursor:5,attempt_id:"attempt",kind:"metrics" as const,captured_at:"2026-08-12T10:01:05Z",metrics:[{cursor:5,attempt_id:"attempt",name:"loss",value:1,step:2,captured_at:"2026-08-12T10:01:05Z"}]};
    const nextMetrics=appendMetricUpdate(metrics,metricUpdate)!;
    expect(nextMetrics.cursor).toBe(5);expect(nextMetrics.series[0]).toMatchObject({last:1,min:1,max:2,sample_count:2});expect(nextMetrics.series[0].points).toHaveLength(1);
    expect(appendMetricUpdate(nextMetrics,metricUpdate)).toBe(nextMetrics);
    const resources={attempt_id:"attempt",cursor:5,from:metrics.from,to:metrics.to,resolution_seconds:5,truncated:false,points:[]};
    const resourceUpdate={cursor:6,attempt_id:"attempt",kind:"resources" as const,captured_at:"2026-08-12T10:01:10Z",resource:{cursor:6,attempt_id:"attempt",captured_at:"2026-08-12T10:01:10Z",resolution_seconds:5,sample_count:1,cpu_millis:500,memory_bytes:1024}};
    expect(appendResourceUpdate(resources,resourceUpdate)?.points).toHaveLength(1);
  });
});
