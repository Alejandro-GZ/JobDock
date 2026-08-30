// @vitest-environment jsdom

import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {MemoryRouter} from "react-router-dom";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";
import {NewJob} from "./new-job";
import type {Node,Secret} from "@/types";

const {inventory,secretInventory}=vi.hoisted(()=>({inventory:[] as Node[],secretInventory:[] as Secret[]}));
vi.mock("@/api",()=>({api:{nodes:async()=>inventory,secrets:async()=>secretInventory}}));

describe("NewJob execution source",()=>{
  beforeEach(()=>{inventory.splice(0);secretInventory.splice(0);vi.stubGlobal("ResizeObserver",class{observe(){}unobserve(){}disconnect(){}})});
  afterEach(()=>{cleanup();vi.unstubAllGlobals()});
  it("offers Auto, Dockerfile, and OCI with progressive configuration",async()=>{
    const user=userEvent.setup(),client=new QueryClient({defaultOptions:{queries:{retry:false}}});
    render(<QueryClientProvider client={client}><MemoryRouter><NewJob/></MemoryRouter></QueryClientProvider>);
    const automatic=screen.getByRole("button",{name:/Project \(Auto\)/});
    expect(automatic.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("Recommended")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:/Dockerfile/}));
    expect(screen.getByLabelText("Build context")).toBeTruthy();
    expect(screen.getByLabelText("Dockerfile path")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:/OCI image/}));
    expect(screen.getByLabelText("OCI image")).toBeTruthy();
    expect(screen.queryByLabelText("Build context")).toBeNull();
  });
  it("selects exact hardware and converts memory units",async()=>{
    inventory.push({id:"node-a",name:"GPU host",status:"ONLINE",agent_version:"test",architecture:"amd64",docker_version:"test",cpu_total_millis:8000,cpu_allocated_millis:0,memory_total_bytes:16*1024**3,memory_allocated_bytes:0,workspace_free_bytes:20*1024**3,labels:{zone:"lab"},capabilities:["cpu_package_affinity"],cpu_packages:[{id:"0",model:"Test CPU",physical_cores:4,logical_cpus:[0,1,2,3,4,5,6,7],total_millis:8000,allocated_millis:0}],gpus:[{uuid:"GPU-A",model:"RTX 4060",vram_bytes:8*1024**3,allocated:true}],gpu_discovery:{status:"available"},last_heartbeat:new Date().toISOString()},{id:"node-b",name:"Other host",status:"ONLINE",agent_version:"test",architecture:"amd64",docker_version:"test",cpu_total_millis:4000,cpu_allocated_millis:0,memory_total_bytes:8*1024**3,memory_allocated_bytes:0,workspace_free_bytes:20*1024**3,labels:{},capabilities:[],cpu_packages:[],gpus:[{uuid:"GPU-B",model:"RTX 3060",vram_bytes:12*1024**3,allocated:false}],gpu_discovery:{status:"available"},last_heartbeat:new Date().toISOString()});
    const user=userEvent.setup(),client=new QueryClient({defaultOptions:{queries:{retry:false}}});
    render(<QueryClientProvider client={client}><MemoryRouter><NewJob/></MemoryRouter></QueryClientProvider>);
    await user.click(screen.getByRole("button",{name:/OCI image/}));
    await user.type(screen.getByLabelText("Job name"),"hardware job");
    await user.type(screen.getByLabelText("OCI image"),"alpine:3");
    await user.click(screen.getByRole("button",{name:/Continue/}));
    const gpu=await screen.findByRole("button",{name:/RTX 4060/});
    await user.click(gpu);
    expect(gpu.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button",{name:/RTX 3060/}).hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Selected GPU is currently allocated")).toBeTruthy();
    await user.selectOptions(screen.getByLabelText("Memory unit"),"MiB");
    expect((screen.getByLabelText("Memory value") as HTMLInputElement).value).toBe("1024");
    expect(screen.queryByLabelText("GPU count")).toBeNull();
  });
  it("offers generic secrets to job configuration while keeping registry credentials separate",async()=>{
    secretInventory.push({id:"one",name:"dummy-api-token",kind:"generic",created_at:new Date().toISOString()},{id:"two",name:"dummy-config-json",kind:"generic",created_at:new Date().toISOString()},{id:"registry",name:"dummy-registry",kind:"registry",created_at:new Date().toISOString()});
    const user=userEvent.setup(),client=new QueryClient({defaultOptions:{queries:{retry:false}}});
    render(<QueryClientProvider client={client}><MemoryRouter><NewJob/></MemoryRouter></QueryClientProvider>);
    await user.click(screen.getByRole("button",{name:/OCI image/}));
    await user.type(screen.getByLabelText("Job name"),"secret job");
    await user.type(screen.getByLabelText("OCI image"),"alpine:3");
    await user.click(screen.getByRole("button",{name:/Continue/}));
    await user.click(screen.getByRole("button",{name:/Continue/}));
    await user.click(screen.getByRole("button",{name:"Add secret"}));
    const selector=screen.getByLabelText("Secret") as HTMLSelectElement;
    expect(Array.from(selector.options).map(option=>option.text)).toEqual(["Select secret","dummy-api-token","dummy-config-json"]);
  });
});
