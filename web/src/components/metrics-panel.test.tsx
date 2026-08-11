// @vitest-environment jsdom

import { act } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MetricsPanel } from "./metrics-panel";
import type { Job, SeriesUpdate } from "@/types";

class FakeEventSource {
  static instance: FakeEventSource;
  onopen: (()=>void)|null=null;onerror: (()=>void)|null=null;
  listener?: (event: MessageEvent)=>void;
  constructor(public url:string){FakeEventSource.instance=this}
  addEventListener(type:string,listener:EventListener){if(type==="series")this.listener=listener as (event:MessageEvent)=>void}
  close=vi.fn();
  emit(update:SeriesUpdate){this.listener?.(new MessageEvent("series",{data:JSON.stringify(update)}))}
}

const job:Job={id:"job",owner_id:"owner",attempt_id:"attempt",status:"RUNNING",desired_status:"RUNNING",observed_status:"RUNNING",created_at:"2026-08-12T10:00:00Z",started_at:"2026-08-12T10:00:00Z",version:1,spec:{name:"live",image:"alpine",command:[],environment:{},secret_refs:[],resources:{cpu_millis:1000,memory_bytes:1024,gpu:{count:0,min_vram_bytes:0}},labels:{},node_selector:{}}};

describe("MetricsPanel live updates",()=>{
  afterEach(()=>vi.unstubAllGlobals());
  it("merges an SSE delta without requesting history again",async()=>{
    const fetchMock=vi.fn((input:string|URL|Request)=>{const path=String(input),body=path.includes("/metrics?")?{attempt_id:"attempt",cursor:4,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:0,truncated:false,series:[{name:"loss",points:[{cursor:4,captured_at:"2026-08-12T10:01:00Z",value:2,sample_count:1}],last:2,min:2,max:2,sample_count:1}]}:{attempt_id:"attempt",cursor:4,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:5,truncated:false,points:[]};return Promise.resolve(new Response(JSON.stringify(body),{status:200,headers:{"Content-Type":"application/json"}}))});
    vi.stubGlobal("fetch",fetchMock);vi.stubGlobal("EventSource",FakeEventSource);
    const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
    render(<QueryClientProvider client={client}><MetricsPanel job={job}/></QueryClientProvider>);
    await waitFor(()=>expect(FakeEventSource.instance.url).toContain("after=4"));
    expect(screen.getByText("last 2")).toBeTruthy();const requests=fetchMock.mock.calls.length;
    act(()=>FakeEventSource.instance.emit({cursor:5,attempt_id:"attempt",kind:"metrics",captured_at:"2026-08-12T10:01:05Z",metrics:[{cursor:5,attempt_id:"attempt",name:"loss",value:1,captured_at:"2026-08-12T10:01:05Z"}]}));
    await waitFor(()=>expect(screen.getByText("last 1")).toBeTruthy());expect(fetchMock).toHaveBeenCalledTimes(requests);
  });
});
