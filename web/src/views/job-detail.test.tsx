// @vitest-environment jsdom

import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {MemoryRouter,Route,Routes} from "react-router-dom";
import {afterEach,describe,expect,it,vi} from "vitest";
import {TooltipProvider} from "@/components/ui/tooltip";
import {JobDetail} from "./job-detail";
import type {Job,JobAttempt,Node,User} from "@/types";

const digest="a".repeat(64),attempt:JobAttempt={id:"attempt-1",job_id:"job-1",attempt_number:1,node_id:"node-1",status:"SUCCEEDED",image_digest:`sha256:${digest}`,exit_code:0,outputs:[{path:"models/final model.pt",size:12,sha256:digest}],created_at:"2026-08-13T10:00:00Z",started_at:"2026-08-13T10:00:01Z",finished_at:"2026-08-13T10:00:02Z"};
const job:Job={id:"job-1",owner_id:"user-1",attempt_id:attempt.id,assigned_node_id:"node-1",status:"SUCCEEDED",desired_status:"SUCCEEDED",observed_status:"SUCCEEDED",created_at:attempt.created_at,started_at:attempt.started_at,finished_at:attempt.finished_at,version:2,spec:{name:"Training",image:`jobdock://build/build-1@sha256:${digest}`,command:["python","train.py"],environment:{},secret_refs:[],resources:{cpu_millis:2000,memory_bytes:2147483648,gpu:{count:1,min_vram_bytes:0}},labels:{},node_selector:{}}};
const node:Node={id:"node-1",name:"gpu-worker",status:"ONLINE",agent_version:"test",architecture:"amd64",docker_version:"test",cpu_total_millis:8000,cpu_allocated_millis:2000,memory_total_bytes:17179869184,memory_allocated_bytes:2147483648,workspace_free_bytes:1,labels:{},gpus:[{uuid:"gpu",model:"test",vram_bytes:1,allocated:true}],gpu_discovery:{status:"available"},last_heartbeat:"2026-08-13T10:00:00Z"};

describe("JobDetail",()=>{
  afterEach(()=>{cleanup();vi.unstubAllGlobals()});
  it("uses Overview/Metrics/Misc navigation and keeps attempt actions in their new locations",async()=>{
    vi.stubGlobal("fetch",vi.fn((input:string|URL|Request)=>{const path=String(input);let body:unknown;if(path.endsWith("/jobs/job-1"))body=job;else if(path.endsWith("/jobs/job-1/attempts"))body={items:[attempt]};else if(path.endsWith("/nodes"))body={items:[node]};else if(path.endsWith("/builds/build-1"))body={id:"build-1",owner_id:"user-1",name:"Training image",mode:"RAILPACK",status:"SUCCEEDED",source:{filename:"source.zip",size:1,sha256:digest},artifact_available:true,created_at:job.created_at,version:1};else if(path.includes("/dashboard"))body={schema_version:1,widgets:[]};else if(path.includes("/metrics/catalog"))body={attempt_id:attempt.id,items:[]};else if(path.includes("/metrics?"))body={attempt_id:attempt.id,cursor:0,series:[]};else if(path.includes("/resources?"))body={attempt_id:attempt.id,cursor:0,points:[]};else if(path.includes("/progress?"))body={attempt_id:attempt.id,milestones:[],reached:[]};else body={attempt_id:attempt.id,items:[]};return Promise.resolve(new Response(JSON.stringify(body),{status:200,headers:{"Content-Type":"application/json"}}))}));
    const client=new QueryClient({defaultOptions:{queries:{retry:false}}}),user:User={id:"user-1",username:"owner",role:"member",created_at:job.created_at};render(<QueryClientProvider client={client}><TooltipProvider><MemoryRouter initialEntries={["/jobs/job-1"]}><Routes><Route path="/jobs/:id" element={<JobDetail user={user}/>}/></Routes></MemoryRouter></TooltipProvider></QueryClientProvider>);
    expect(await screen.findByRole("heading",{name:"Training"})).toBeTruthy();expect(screen.getByRole("tab",{name:"Overview"})).toBeTruthy();expect(screen.getByRole("tab",{name:"Metrics"})).toBeTruthy();expect(screen.getByRole("tab",{name:"Misc"})).toBeTruthy();expect(screen.queryByRole("tab",{name:"Logs"})).toBeNull();expect(screen.queryByRole("tab",{name:"Events"})).toBeNull();
    expect((await screen.findByRole("link",{name:"Training image"})).getAttribute("href")).toBe("/builds/build-1");expect(screen.getByRole("link",{name:"gpu-worker"}).getAttribute("href")).toContain("/nodes?node=node-1");expect(screen.getByRole("link",{name:"Download models/final model.pt"}).getAttribute("href")).toContain("models/final%20model.pt");
    await userEvent.click(screen.getByRole("button",{name:"Open attempt history"}));expect(await screen.findByRole("dialog")).toBeTruthy();expect(screen.getByText("#1")).toBeTruthy();await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("tab",{name:"Metrics"}));expect(await screen.findByRole("button",{name:"Edit dashboard"})).toBeTruthy();await userEvent.click(screen.getByRole("button",{name:"Edit dashboard"}));expect(screen.getByRole("button",{name:"Done"})).toBeTruthy();
    await userEvent.click(screen.getByRole("tab",{name:"Misc"}));await waitFor(()=>expect(screen.getByRole("link",{name:"Metrics CSV"})).toBeTruthy());expect(screen.getByRole("link",{name:"Resources CSV"})).toBeTruthy();expect(screen.getByRole("link",{name:"Events JSON"})).toBeTruthy();expect(screen.getByRole("button",{name:"Delete job"})).toBeTruthy();
  });
});
