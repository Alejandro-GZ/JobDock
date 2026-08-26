// @vitest-environment jsdom

import { useState } from "react";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import type { DashboardTemplate, DashboardTemplateOverride, DashboardTemplateReference, DashboardTemplateResolution, DashboardWidget } from "@/lib/dashboard-widgets";
import { DashboardTemplatePicker } from "./dashboard-template-picker";

Object.defineProperties(HTMLElement.prototype, {
  hasPointerCapture: { configurable: true, value: () => false },
  setPointerCapture: { configurable: true, value: () => undefined },
  releasePointerCapture: { configurable: true, value: () => undefined },
  scrollIntoView: { configurable: true, value: () => undefined },
});

const template:DashboardTemplate={id:"training",name:"Training",description:"Semantic training signals.",category:"general",schema_version:1,version:3,widgets:[{id:"loss",type:"lineplot",size:{columns:12,rows:4},position:{x:0,y:0},slots:[{id:"loss",required_tags:["metric:loss"],source_types:["metric"],cardinality:{min:1,max:1}}]}]};
const previous:DashboardWidget[]=[{id:"previous",type:"logs",size:{columns:12,rows:6},position:{x:0,y:0},sources:[{kind:"log",name:"stdout"}]}];
const applied:DashboardWidget[]=[{id:"loss",type:"lineplot",size:{columns:12,rows:4},position:{x:0,y:0},sources:[{kind:"metric",name:"objective_b"}]}];

describe("DashboardTemplatePicker",()=>{
  afterEach(()=>{cleanup();vi.restoreAllMocks()});
  it("previews diagnostics, resolves ambiguity, confirms replacement, and restores the previous dashboard",async()=>{
    vi.spyOn(api,"dashboardTemplates").mockResolvedValue([template]);
    vi.spyOn(api,"dashboardTemplateMatches").mockResolvedValue([{template_id:"training",compatibility:"incompatible",applicable:false,missing_required:0,ambiguous_sources:1}]);
    vi.spyOn(api,"resolveDashboardTemplate").mockImplementation(async(_job,_template,_attempt,overrides:DashboardTemplateOverride[]=[])=>resolution(overrides.length>0));
    const onApply=vi.fn(async()=>undefined),client=new QueryClient({defaultOptions:{queries:{retry:false}}}),user=userEvent.setup();
    render(<QueryClientProvider client={client}><Harness onApply={onApply}/></QueryClientProvider>);
    await user.click(screen.getByRole("button",{name:"Open templates"}));
    expect(await screen.findByText("Semantic training signals.")).toBeTruthy();
    expect(screen.getByText("Template v3 · schema v1")).toBeTruthy();
    expect(await screen.findByText("Layout preview")).toBeTruthy();
    expect(screen.getByText("incompatible")).toBeTruthy();
    expect(screen.getByText("2 matching sources require a choice")).toBeTruthy();
    await user.click(screen.getByRole("combobox",{name:"Resolve loss"}));
    await user.click(await screen.findByRole("option",{name:"objective_b"}));
    await waitFor(()=>expect(screen.getAllByText("objective_b").length).toBeGreaterThan(0));
    await user.click(screen.getByRole("button",{name:"Apply template"}));
    expect(await screen.findByText("Replace the current dashboard?")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:"Replace dashboard"}));
    await waitFor(()=>expect(onApply).toHaveBeenCalledWith(applied,{template_id:"training",template_version:3,schema_version:1}));
    await user.click(screen.getByRole("button",{name:"Open templates"}));
    await user.click(await screen.findByRole("button",{name:"Restore previous dashboard"}));
    await waitFor(()=>expect(onApply).toHaveBeenLastCalledWith(previous,null));
  });
  it("shows a controlled fallback and blocks an incompatible template",async()=>{
    vi.spyOn(api,"dashboardTemplates").mockResolvedValue([template]);
    vi.spyOn(api,"dashboardTemplateMatches").mockResolvedValue([{template_id:"training",compatibility:"incompatible",applicable:false,missing_required:1,ambiguous_sources:0}]);
    vi.spyOn(api,"resolveDashboardTemplate").mockResolvedValue({template_id:"training",schema_version:99,template_version:3,attempt_id:"attempt",compatibility:"incompatible",fallback_reason:"unsupported_schema_version",widgets:[],widget_results:[],slot_results:[]});
    const client=new QueryClient({defaultOptions:{queries:{retry:false}}}),user=userEvent.setup();
    render(<QueryClientProvider client={client}><Harness onApply={vi.fn(async()=>undefined)}/></QueryClientProvider>);
    await user.click(screen.getByRole("button",{name:"Open templates"}));
    expect(await screen.findByText(/cannot be applied safely: unsupported schema version/)).toBeTruthy();
    expect((screen.getByRole("button",{name:"Apply template"}) as HTMLButtonElement).disabled).toBe(true);
  });
  it("groups, searches, and filters a large catalog without the old selector copy",async()=>{
    const vision:DashboardTemplate={...template,id:"vision",name:"Object detection",description:"Detection metrics.",category:"computer-vision"};
    vi.spyOn(api,"dashboardTemplates").mockResolvedValue([template,vision]);
    vi.spyOn(api,"dashboardTemplateMatches").mockResolvedValue([{template_id:"training",compatibility:"partially_compatible",applicable:true,missing_required:0,ambiguous_sources:0},{template_id:"vision",compatibility:"incompatible",applicable:false,missing_required:1,ambiguous_sources:0}]);
    vi.spyOn(api,"resolveDashboardTemplate").mockResolvedValue(resolution(true));
    const client=new QueryClient({defaultOptions:{queries:{retry:false}}}),user=userEvent.setup();render(<QueryClientProvider client={client}><Harness onApply={vi.fn(async()=>undefined)}/></QueryClientProvider>);
    await user.click(screen.getByRole("button",{name:"Open templates"}));
    expect(await screen.findByText("General")).toBeTruthy();expect(screen.getByText("Computer vision")).toBeTruthy();expect(screen.queryByRole("combobox",{name:"Dashboard template"})).toBeNull();expect(screen.queryByText("Preview semantic matches before replacing the current editable layout.")).toBeNull();
    await user.click(screen.getByRole("button",{name:"Applicable"}));expect(screen.queryByText("Object detection")).toBeNull();
    await user.click(screen.getByRole("button",{name:"Applicable"}));await user.type(screen.getByRole("textbox",{name:"Search templates"}),"detection");expect((await screen.findAllByText("Object detection")).length).toBeGreaterThan(0);expect(screen.queryByText("Training",{selector:"button span"})).toBeNull();
  });
  it("creates a new dashboard with the selected template name by default",async()=>{
    vi.spyOn(api,"dashboardTemplates").mockResolvedValue([template]);
    vi.spyOn(api,"dashboardTemplateMatches").mockResolvedValue([{template_id:"training",compatibility:"compatible",applicable:true,missing_required:0,ambiguous_sources:0}]);
    vi.spyOn(api,"resolveDashboardTemplate").mockResolvedValue(resolution(true));
    const onCreate=vi.fn(async()=>undefined),client=new QueryClient({defaultOptions:{queries:{retry:false}}}),user=userEvent.setup();
    render(<QueryClientProvider client={client}><CreateHarness onCreate={onCreate}/></QueryClientProvider>);
    await user.click(screen.getByRole("button",{name:"Create from template"}));
    const name=await screen.findByRole("textbox",{name:"Dashboard name"});
    await waitFor(()=>expect((name as HTMLInputElement).value).toBe("Training"));
    await user.click(screen.getByRole("button",{name:"Create dashboard"}));
    await waitFor(()=>expect(onCreate).toHaveBeenCalledWith("Training",applied,{template_id:"training",template_version:3,schema_version:1}));
  });
});

function Harness({onApply}:{onApply:(widgets:DashboardWidget[],materializedFrom:DashboardTemplateReference|null)=>Promise<void>}){const[open,setOpen]=useState(false);return <><Button onClick={()=>setOpen(true)}>Open templates</Button><DashboardTemplatePicker open={open} onOpenChange={setOpen} jobID="job" attemptID="attempt" currentWidgets={previous} onApply={onApply}/></>}
function CreateHarness({onCreate}:{onCreate:(name:string,widgets:DashboardWidget[],materializedFrom:DashboardTemplateReference)=>Promise<void>}){const[open,setOpen]=useState(false);return <><Button onClick={()=>setOpen(true)}>Create from template</Button><DashboardTemplatePicker open={open} onOpenChange={setOpen} jobID="job" attemptID="attempt" mode="create" onCreate={onCreate}/></>}
function resolution(overridden:boolean):DashboardTemplateResolution{return{template_id:"training",schema_version:1,template_version:3,attempt_id:"attempt",compatibility:overridden?"compatible":"incompatible",widgets:overridden?applied:[],widget_results:[{widget_id:"loss",status:overridden?"resolved":"unresolved"}],slot_results:[{widget_id:"loss",slot_id:"loss",status:overridden?"resolved":"ambiguous",candidates:[{kind:"metric",name:"objective_a"},{kind:"metric",name:"objective_b"}],selected:overridden?[{kind:"metric",name:"objective_b"}]:[]}]}}
