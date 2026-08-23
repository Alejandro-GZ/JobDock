// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ObservabilityDashboard } from "./observability-dashboard";
import { TooltipProvider } from "@/components/ui/tooltip";

const wrap=(node:React.ReactNode)=><TooltipProvider>{node}</TooltipProvider>;
describe("ObservabilityDashboard",()=>{
  afterEach(()=>{cleanup();vi.unstubAllGlobals()});
  it("keeps layout controls in edit mode and adds widgets by dropping from the library",async()=>{
    const changed=vi.fn();render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[]} matrices={[]} markers={[]} initialWidgets={[]} onWidgetsChange={changed}/>));
    expect(await screen.findByText("Drag a widget from the library into this area.")).toBeTruthy();
    const palette=screen.getByText("Bar plot").closest("[draggable=true]")!,zone=screen.getByLabelText("Metrics dashboard").querySelector(".relative.min-w-0")!;
    fireEvent.dragStart(palette,{dataTransfer:dataTransfer()});fireEvent.dragOver(zone,{dataTransfer:dataTransfer()});fireEvent.drop(zone,{dataTransfer:dataTransfer()});
    await waitFor(()=>expect(document.querySelector('[data-widget-type="barplot"]')).toBeTruthy());expect(screen.getByRole("button",{name:"Remove widget"})).toBeTruthy();const handle=screen.getByRole("button",{name:"Drag to resize widget"});fireEvent.pointerDown(handle,{clientX:0,clientY:0,pointerId:1});fireEvent.pointerMove(window,{clientX:120,clientY:90,pointerId:1});await waitFor(()=>expect(document.querySelector<HTMLElement>('[data-widget-type="barplot"]')?.dataset.size).toBe("7x4"));fireEvent.pointerUp(window,{clientX:120,clientY:90,pointerId:1});expect(document.querySelector<HTMLElement>('[data-widget-type="barplot"]')?.dataset.size).toBe("7x4");
  });
  it("renders real values only outside edit mode",async()=>{
    const source={kind:"metric" as const,name:"loss",title:"loss",unit:"ratio",points:[{timestamp:1_000,value:.5}]},saved=[{id:"loss",type:"lineplot" as const,size:{columns:2,rows:1},position:{x:0,y:0},sources:[{kind:"metric" as const,name:"loss"}],grid_columns:4 as const}];
    const {rerender}=render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));expect(await screen.findByRole("img",{name:/Line plot lineplot/})).toBeTruthy();
    rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));expect(screen.queryByRole("img",{name:/Line plot lineplot/})).toBeNull();expect(screen.getByText("metric / loss")).toBeTruthy();
  });
  it("preconfigures declared phase sources and activates them when data arrives",async()=>{
    const waiting={kind:"metric" as const,name:"validation_loss",title:"validation_loss",unit:"ratio",points:[],phase:"validation",declared:true,observed:false},saved=[{id:"future-loss",type:"lineplot" as const,size:{columns:6,rows:3},position:{x:0,y:0},sources:[{kind:"metric" as const,name:"validation_loss"}],grid_columns:12 as const}];
    const {rerender}=render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[waiting]} matrices={[]} markers={[]} initialWidgets={saved}/>));
    expect(await screen.findByText("Waiting for data")).toBeTruthy();expect(screen.getByText("Phase: validation")).toBeTruthy();
    rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[{...waiting,observed:true,points:[{timestamp:1_000,value:.4}]}]} matrices={[]} markers={[]} initialWidgets={saved}/>));
    expect(await screen.findByRole("img",{name:/Line plot lineplot with 1 points/})).toBeTruthy();expect(screen.queryByText("Waiting for data")).toBeNull();
  });
  it("offers declared matrix sources before observations exist",async()=>{
    const user=userEvent.setup(),descriptors=[{name:"validation_confusion",type:"matrix",phase:"validation",declared:true,observed:false}],saved=[{id:"matrix",type:"confusion_matrix" as const,size:{columns:6,rows:3},position:{x:0,y:0},sources:[{kind:"matrix" as const,name:"validation_confusion"}],grid_columns:12 as const}];
    const {rerender}=render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[]} observableSources={descriptors} matrices={[]} markers={[]} initialWidgets={saved}/>));
    await user.click(screen.getByRole("button",{name:"Configure widget"}));expect(screen.getByRole("combobox",{name:"Source"}).textContent).toContain("validation · validation_confusion");
    await user.click(screen.getByRole("button",{name:"Cancel"}));rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[]} observableSources={descriptors} matrices={[]} markers={[]} initialWidgets={saved}/>));expect(await screen.findByText("Waiting for data")).toBeTruthy();
  });
  it("materializes a template replacement as an ordinary editable dashboard",async()=>{
    const source={kind:"metric" as const,name:"loss",title:"loss",unit:"ratio",points:[]},saved=[{id:"logs",type:"logs" as const,size:{columns:4,rows:2},position:{x:0,y:0},sources:[{kind:"log" as const,name:"stdout"}],grid_columns:4 as const}],replacement=[{id:"loss",type:"lineplot" as const,size:{columns:2,rows:1},position:{x:0,y:0},sources:[{kind:"metric" as const,name:"loss"}],grid_columns:4 as const}],changed=vi.fn();
    const {rerender}=render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved} onWidgetsChange={changed}/>));
    expect((await screen.findAllByText("log / stdout")).length).toBeGreaterThan(0);
    rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved} replacement={{key:"template-1",widgets:replacement}} onWidgetsChange={changed}/>));
    expect(await screen.findByText("metric / loss")).toBeTruthy();
    expect(screen.getByRole("button",{name:"Configure widget"})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Remove widget"})).toBeTruthy();
    expect(changed).not.toHaveBeenCalled();
  });
  it("previews tile reflow during drag and commits it only on drop",async()=>{
    const source={kind:"metric" as const,name:"loss",title:"loss",unit:"ratio",points:[]},saved=["first","second"].map((id,index)=>({id,type:"lineplot" as const,size:{columns:2,rows:1},position:{x:index*2,y:0},sources:[{kind:"metric" as const,name:"loss"}],grid_columns:4 as const}));
    render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));await waitFor(()=>expect(document.querySelectorAll("[data-widget-id]")).toHaveLength(2));const transfer=dataTransfer(),tiles=()=>[...document.querySelectorAll<HTMLElement>("[data-widget-id]")];fireEvent.dragStart(tiles()[1].querySelector("section")!,{dataTransfer:transfer});fireEvent.dragOver(tiles()[0],{dataTransfer:transfer});await waitFor(()=>expect(tiles().map(tile=>tile.dataset.widgetId)).toEqual(["second","first"]));fireEvent.drop(tiles()[0],{dataTransfer:transfer});expect(tiles().map(tile=>tile.dataset.widgetId)).toEqual(["second","first"]);
  });
  it("configures widget data and time range from its editing shell",async()=>{
    const user=userEvent.setup(),changed=vi.fn(),sources=[{kind:"metric" as const,name:"loss",title:"loss",unit:"ratio",points:[{timestamp:1,value:.8,step:1}]},{kind:"metric" as const,name:"duration",title:"duration",unit:"seconds",points:[{timestamp:1,value:8,step:1}]}],saved=[{id:"loss",type:"lineplot" as const,size:{columns:2,rows:1},position:{x:0,y:0},sources:[{kind:"metric" as const,name:"loss"}],grid_columns:4 as const}];
    render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={sources} matrices={[]} markers={[]} initialWidgets={saved} onWidgetsChange={changed}/>));await user.click(screen.getByRole("button",{name:"Configure widget"}));expect(screen.getByRole("combobox",{name:"Time range"})).toBeTruthy();await user.type(screen.getByLabelText(/Title/),"Training signals");await user.click(screen.getByRole("checkbox",{name:"duration (seconds)"}));await user.click(screen.getByRole("button",{name:"Apply"}));expect(await screen.findByText("metric / loss · metric / duration")).toBeTruthy();await waitFor(()=>expect(changed.mock.calls.at(-1)?.[0]?.[0]?.title).toBe("Training signals"));
  });
  it("renders a gauge and exposes its fixed maximum in edit mode",async()=>{
    const user=userEvent.setup(),source={kind:"metric" as const,name:"temperature",title:"Temperature",unit:"°C",points:[{timestamp:1,value:20},{timestamp:2,value:40}]},saved=[{id:"gauge",type:"gauge" as const,size:{columns:6,rows:3},position:{x:0,y:0},sources:[{kind:"metric" as const,name:"temperature"}],grid_columns:12 as const,gauge_max_mode:"fixed" as const,gauge_max_value:100}];
    const {rerender}=render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));expect(await screen.findByRole("meter",{name:"Temperature utilization"})).toBeTruthy();
    rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));await user.click(screen.getByRole("button",{name:"Configure widget"}));expect(screen.getByRole("combobox",{name:"Gauge maximum"}).textContent).toContain("Fixed value");expect((screen.getByLabelText("Fixed maximum") as HTMLInputElement).value).toBe("100");
  });
  it("shows a single combined Logs console with stream selection beside its title",async()=>{
    class EventSourceStub{onopen=null;onerror=null;addEventListener(){}close(){}}vi.stubGlobal("EventSource",EventSourceStub);const user=userEvent.setup(),saved=[{id:"logs",type:"logs" as const,size:{columns:4,rows:2},position:{x:0,y:0},sources:[{kind:"log" as const,name:"stdout"}],grid_columns:4 as const}];
    render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[]} matrices={[]} markers={[]} initialWidgets={saved}/>));expect((await screen.findAllByText("Logs")).length).toBeGreaterThan(0);expect(screen.queryByText("stdout")).toBeNull();await user.click(screen.getByRole("button",{name:"Configure widget"}));expect((screen.getByRole("checkbox",{name:"stdout"}) as HTMLInputElement).checked).toBe(true);await user.click(screen.getByRole("checkbox",{name:"stderr"}));await user.click(screen.getByRole("button",{name:"Apply"}));
  });
});
function dataTransfer(){const values=new Map<string,string>();return{effectAllowed:"all",dropEffect:"move",files:[],items:[],types:[],setData:(key:string,value:string)=>values.set(key,value),getData:(key:string)=>values.get(key)??"",clearData:()=>values.clear(),setDragImage:()=>undefined}}
