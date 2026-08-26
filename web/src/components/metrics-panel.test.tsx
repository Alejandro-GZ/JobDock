// @vitest-environment jsdom

import { act } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MetricsPanel } from "./metrics-panel";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { Job, SeriesUpdate } from "@/types";

Object.defineProperties(HTMLElement.prototype, {
  hasPointerCapture: { configurable: true, value: () => false },
  setPointerCapture: { configurable: true, value: () => undefined },
  releasePointerCapture: { configurable: true, value: () => undefined },
  scrollIntoView: { configurable: true, value: () => undefined },
});

class FakeEventSource {
  static instance: FakeEventSource;
  onopen: (()=>void)|null=null;onerror: (()=>void)|null=null;
  listener?: (event: MessageEvent)=>void;
  constructor(public url:string){if(url.includes("/series/stream"))FakeEventSource.instance=this}
  addEventListener(type:string,listener:EventListener){if(type==="series")this.listener=listener as (event:MessageEvent)=>void}
  close=vi.fn();
  emit(update:SeriesUpdate){this.listener?.(new MessageEvent("series",{data:JSON.stringify(update)}))}
}

const job:Job={id:"job",owner_id:"owner",attempt_id:"attempt",status:"RUNNING",desired_status:"RUNNING",observed_status:"RUNNING",created_at:"2026-08-12T10:00:00Z",started_at:"2026-08-12T10:00:00Z",version:1,spec:{name:"live",image:"alpine",command:[],environment:{},secret_refs:[],resources:{cpu_millis:1000,memory_bytes:1024,gpu:{count:0,min_vram_bytes:0}},labels:{},node_selector:{}}};

describe("MetricsPanel live updates",()=>{
  afterEach(()=>{cleanup();vi.unstubAllGlobals()});
  it("merges an SSE delta without requesting history again",async()=>{
    const fetchMock=vi.fn((input:string|URL|Request)=>{const path=String(input);let body:unknown;if(path.endsWith("/jobs/job/dashboards"))body=dashboardList();else if(path.includes("/dashboards/dashboard-1"))body=dashboardPreference(null);else if(path.includes("/metrics/catalog"))body={attempt_id:"attempt",items:[{name:"loss"},{name:"throughput"}]};else if(path.includes("/metrics?"))body={attempt_id:"attempt",cursor:4,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:0,truncated:false,series:[{name:"loss",points:[{cursor:4,captured_at:"2026-08-12T10:01:00Z",value:2,sample_count:1}],last:2,min:2,max:2,sample_count:1},{name:"throughput",points:[{cursor:4,captured_at:"2026-08-12T10:01:00Z",value:80,sample_count:1}],last:80,min:80,max:80,sample_count:1}]};else if(path.includes("/resources?"))body={attempt_id:"attempt",cursor:4,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:5,truncated:false,points:[]};else if(path.includes("/progress?"))body={attempt_id:"attempt",milestones:[],reached:[]};else if(path.includes("/checkpoints?")||path.includes("/matrices?"))body={attempt_id:"attempt",items:[]};else body={};return Promise.resolve(new Response(JSON.stringify(body),{status:200,headers:{"Content-Type":"application/json"}}))});
    vi.stubGlobal("fetch",fetchMock);vi.stubGlobal("EventSource",FakeEventSource);
    const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
    render(<QueryClientProvider client={client}><TooltipProvider><MetricsPanel job={job}/></TooltipProvider></QueryClientProvider>);
    await waitFor(()=>expect(FakeEventSource.instance.url).toContain("after=4"));
    expect(screen.getAllByRole("img",{name:/lineplot with 1 points/})).toHaveLength(2);const metricRequests=fetchMock.mock.calls.filter(call=>String(call[0]).includes("/metrics?")).length;
    act(()=>FakeEventSource.instance.emit({cursor:5,attempt_id:"attempt",kind:"metrics",captured_at:"2026-08-12T10:01:05Z",metrics:[{cursor:5,attempt_id:"attempt",name:"loss",value:1,captured_at:"2026-08-12T10:01:05Z"}]}));
    await waitFor(()=>expect(screen.getByRole("img",{name:/lineplot with 2 points/})).toBeTruthy());expect(fetchMock.mock.calls.filter(call=>String(call[0]).includes("/metrics?"))).toHaveLength(metricRequests);
  });

  it("loads only metric histories referenced by a saved dashboard",async()=>{
    const widgets=[{id:"saved-loss",type:"lineplot",size:{columns:1,rows:1},position:{x:0,y:0},sources:[{kind:"metric",name:"loss"}],x_axis:"time"}];
    const fetchMock=vi.fn((input:string|URL|Request)=>{const path=String(input);let body:unknown;if(path.endsWith("/jobs/job/dashboards"))body=dashboardList();else if(path.includes("/dashboards/dashboard-1"))body=dashboardPreference(widgets);else if(path.includes("/metrics/catalog"))body={attempt_id:"attempt",items:[{name:"loss"},{name:"throughput"}]};else if(path.includes("/metrics?"))body={attempt_id:"attempt",cursor:0,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:0,truncated:false,series:[]};else if(path.includes("/resources?"))body={attempt_id:"attempt",cursor:0,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:5,truncated:false,points:[]};else if(path.includes("/progress?"))body={attempt_id:"attempt",milestones:[],reached:[]};else body={attempt_id:"attempt",items:[]};return Promise.resolve(new Response(JSON.stringify(body),{status:200,headers:{"Content-Type":"application/json"}}))});
    vi.stubGlobal("fetch",fetchMock);vi.stubGlobal("EventSource",FakeEventSource);const client=new QueryClient({defaultOptions:{queries:{retry:false}}});render(<QueryClientProvider client={client}><TooltipProvider><MetricsPanel job={job}/></TooltipProvider></QueryClientProvider>);
    await waitFor(()=>expect(fetchMock.mock.calls.some(call=>String(call[0]).includes("/metrics?"))).toBe(true));
    const url=String(fetchMock.mock.calls.find(call=>String(call[0]).includes("/metrics?"))?.[0]);expect(url).toContain("name=loss");expect(url).not.toContain("throughput");
  });

  it("switches independent dashboards without refetching an identical metric history",async()=>{
    const items=[{id:"dashboard-1",name:"Training",sort_order:0,is_default:true,created_at:"2026-08-12T10:00:00Z",updated_at:"2026-08-12T10:00:00Z"},{id:"dashboard-2",name:"Validation",sort_order:1,is_default:false,created_at:"2026-08-12T10:00:00Z",updated_at:"2026-08-12T10:00:00Z"}],widgets=[{id:"loss",type:"lineplot",size:{columns:6,rows:3},position:{x:0,y:0},sources:[{kind:"metric",name:"loss"}],x_axis:"time"}];
    let metricRequests=0;const patches:string[]=[];
    const fetchMock=vi.fn((input:string|URL|Request,init?:RequestInit)=>{const path=String(input),method=init?.method??"GET";let body:unknown;if(path.endsWith("/jobs/job/dashboards"))body={items,active_dashboard_id:"dashboard-1",default_dashboard_id:"dashboard-1"};else if(path.includes("/dashboards/dashboard-")){const item=items.find(candidate=>path.endsWith(candidate.id))!;if(method==="PATCH")patches.push(path);body={...item,schema_version:1,widgets,compatibility:"compatible",materialized_from:null}}else if(path.includes("/metrics/catalog"))body={attempt_id:"attempt",items:[{name:"loss"}]};else if(path.includes("/metrics?")){metricRequests++;body={attempt_id:"attempt",cursor:1,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:0,truncated:false,series:[{name:"loss",points:[{cursor:1,captured_at:"2026-08-12T10:01:00Z",value:1,sample_count:1}],last:1,min:1,max:1,sample_count:1}]}}else if(path.includes("/resources?"))body={attempt_id:"attempt",cursor:1,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:5,truncated:false,points:[]};else if(path.includes("/progress?"))body={attempt_id:"attempt",milestones:[],reached:[]};else body={attempt_id:"attempt",items:[]};return Promise.resolve(new Response(JSON.stringify(body),{status:200,headers:{"Content-Type":"application/json"}}))});
    vi.stubGlobal("fetch",fetchMock);vi.stubGlobal("EventSource",FakeEventSource);const client=new QueryClient({defaultOptions:{queries:{retry:false}}});render(<QueryClientProvider client={client}><TooltipProvider><MetricsPanel job={job}/></TooltipProvider></QueryClientProvider>);
    const user=userEvent.setup(),training=await screen.findByRole("tab",{name:"Training"}),validation=screen.getByRole("tab",{name:"Validation"});await waitFor(()=>expect(metricRequests).toBe(1));expect(training.getAttribute("aria-selected")).toBe("true");training.focus();await user.keyboard("[ArrowDown]");await waitFor(()=>expect(patches).toHaveLength(1));expect(validation.getAttribute("aria-selected")).toBe("true");expect(document.activeElement).toBe(validation);expect(metricRequests).toBe(1);
    expect(screen.queryByRole("button",{name:/dashboard rail/i})).toBeNull();expect(screen.getByLabelText("Training is default")).toBeTruthy();
    await user.click(validation);expect(await screen.findByRole("menuitem",{name:"Rename"})).toBeTruthy();await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button",{name:"Create dashboard"}));expect(await screen.findByRole("heading",{name:"Create dashboard"})).toBeTruthy();expect(screen.getByRole("button",{name:"From template"})).toBeTruthy();expect(screen.getByRole("button",{name:"Create blank"})).toBeTruthy();
  });

  it("preserves widget sources across consecutive dashboard palette saves",async()=>{
    const widgets=[{id:"loss",type:"lineplot",size:{columns:6,rows:3},position:{x:0,y:0},sources:[{kind:"metric",name:"loss"}],x_axis:"time"}],saved:Array<{widgets:unknown[];appearance?:unknown}>=[];
    const fetchMock=vi.fn((input:string|URL|Request,init?:RequestInit)=>{const path=String(input),method=init?.method??"GET";let body:unknown;if(path.endsWith("/jobs/job/dashboards"))body=dashboardList();else if(path.includes("/dashboards/dashboard-1")){if(method==="PUT"){const payload=JSON.parse(String(init?.body)) as {widgets:unknown[];appearance?:unknown};saved.push(payload);body={...dashboardPreference(payload.widgets),appearance:payload.appearance}}else body=dashboardPreference(widgets)}else if(path.includes("/metrics/catalog"))body={attempt_id:"attempt",items:[{name:"loss",observed:true}]};else if(path.includes("/metrics?"))body={attempt_id:"attempt",cursor:1,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:0,truncated:false,series:[{name:"loss",points:[{cursor:1,captured_at:"2026-08-12T10:01:00Z",value:1,sample_count:1}],last:1,min:1,max:1,sample_count:1}]};else if(path.includes("/resources?"))body={attempt_id:"attempt",cursor:1,from:"2026-08-12T10:00:00Z",to:"2026-08-12T10:01:00Z",resolution_seconds:5,truncated:false,points:[]};else if(path.includes("/progress?"))body={attempt_id:"attempt",milestones:[],reached:[]};else body={attempt_id:"attempt",items:[]};return Promise.resolve(new Response(JSON.stringify(body),{status:200,headers:{"Content-Type":"application/json"}}))});
    vi.stubGlobal("fetch",fetchMock);vi.stubGlobal("EventSource",FakeEventSource);const client=new QueryClient({defaultOptions:{queries:{retry:false}}});render(<QueryClientProvider client={client}><TooltipProvider><MetricsPanel job={job} editMode/></TooltipProvider></QueryClientProvider>);
    const user=userEvent.setup();await screen.findByText("metric / loss");await user.click(screen.getByRole("button",{name:"JobDock"}));await waitFor(()=>expect(saved).toHaveLength(1),{timeout:2000});await user.click(screen.getByRole("button",{name:"Midnight"}));await waitFor(()=>expect(saved).toHaveLength(2),{timeout:2000});await user.click(screen.getByRole("button",{name:"Reset dashboard palette"}));await waitFor(()=>expect(saved).toHaveLength(3),{timeout:2000});expect(saved.map(item=>item.widgets.length)).toEqual([1,1,1]);expect(saved.map(item=>(item.widgets[0] as typeof widgets[0]).sources[0])).toEqual([{kind:"metric",name:"loss"},{kind:"metric",name:"loss"},{kind:"metric",name:"loss"}]);expect(saved[2].appearance).toBeNull();
  });
});

function dashboardList(){return{items:[{id:"dashboard-1",name:"Dashboard",sort_order:0,is_default:true,created_at:"2026-08-12T10:00:00Z",updated_at:"2026-08-12T10:00:00Z"}],active_dashboard_id:"dashboard-1",default_dashboard_id:"dashboard-1"}}
function dashboardPreference(widgets:unknown){return{...dashboardList().items[0],schema_version:1,widgets,compatibility:"compatible",materialized_from:null}}
