import { describe, expect, it } from "vitest";
import { numericSummary, resourcePoints, seriesQuery, zoomDomain } from "./series";

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
});
