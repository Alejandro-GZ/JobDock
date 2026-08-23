// @vitest-environment jsdom

import { useState } from "react";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import type { DashboardTemplate, DashboardTemplateOverride, DashboardTemplateResolution, DashboardWidget } from "@/lib/dashboard-widgets";
import { DashboardTemplatePicker } from "./dashboard-template-picker";

Object.defineProperties(HTMLElement.prototype, {
  hasPointerCapture: { configurable: true, value: () => false },
  setPointerCapture: { configurable: true, value: () => undefined },
  releasePointerCapture: { configurable: true, value: () => undefined },
  scrollIntoView: { configurable: true, value: () => undefined },
});

const template:DashboardTemplate={id:"training",name:"Training",description:"Semantic training signals.",schema_version:1,widgets:[{id:"loss",type:"lineplot",size:{columns:12,rows:4},position:{x:0,y:0},slots:[{id:"loss",required_tags:["metric:loss"],source_types:["metric"],cardinality:{min:1,max:1}}]}]};
const previous:DashboardWidget[]=[{id:"previous",type:"logs",size:{columns:12,rows:6},position:{x:0,y:0},sources:[{kind:"log",name:"stdout"}]}];
const applied:DashboardWidget[]=[{id:"loss",type:"lineplot",size:{columns:12,rows:4},position:{x:0,y:0},sources:[{kind:"metric",name:"objective_b"}]}];

describe("DashboardTemplatePicker",()=>{
  afterEach(()=>{cleanup();vi.restoreAllMocks()});
  it("previews diagnostics, resolves ambiguity, confirms replacement, and restores the previous dashboard",async()=>{
    vi.spyOn(api,"dashboardTemplates").mockResolvedValue([template]);
    vi.spyOn(api,"resolveDashboardTemplate").mockImplementation(async(_job,_template,_attempt,overrides:DashboardTemplateOverride[]=[])=>resolution(overrides.length>0));
    const onApply=vi.fn(async()=>undefined),client=new QueryClient({defaultOptions:{queries:{retry:false}}}),user=userEvent.setup();
    render(<QueryClientProvider client={client}><Harness onApply={onApply}/></QueryClientProvider>);
    await user.click(screen.getByRole("button",{name:"Open templates"}));
    expect(await screen.findByText("Semantic training signals.")).toBeTruthy();
    expect(screen.getByText("Layout preview")).toBeTruthy();
    expect(screen.getByText("2 matching sources require a choice")).toBeTruthy();
    await user.click(screen.getByRole("combobox",{name:"Resolve loss"}));
    await user.click(await screen.findByRole("option",{name:"objective_b"}));
    await waitFor(()=>expect(screen.getAllByText("objective_b").length).toBeGreaterThan(0));
    await user.click(screen.getByRole("button",{name:"Apply template"}));
    expect(await screen.findByText("Replace the current dashboard?")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:"Replace dashboard"}));
    await waitFor(()=>expect(onApply).toHaveBeenCalledWith(applied));
    await user.click(screen.getByRole("button",{name:"Open templates"}));
    await user.click(await screen.findByRole("button",{name:"Restore previous dashboard"}));
    await waitFor(()=>expect(onApply).toHaveBeenLastCalledWith(previous));
  });
});

function Harness({onApply}:{onApply:(widgets:DashboardWidget[])=>Promise<void>}){const[open,setOpen]=useState(false);return <><Button onClick={()=>setOpen(true)}>Open templates</Button><DashboardTemplatePicker open={open} onOpenChange={setOpen} jobID="job" attemptID="attempt" currentWidgets={previous} onApply={onApply}/></>}
function resolution(overridden:boolean):DashboardTemplateResolution{return{template_id:"training",schema_version:1,attempt_id:"attempt",widgets:overridden?applied:[],widget_results:[{widget_id:"loss",status:overridden?"resolved":"unresolved"}],slot_results:[{widget_id:"loss",slot_id:"loss",status:overridden?"resolved":"ambiguous",candidates:[{kind:"metric",name:"objective_a"},{kind:"metric",name:"objective_b"}],selected:overridden?[{kind:"metric",name:"objective_b"}]:[]}]}}
