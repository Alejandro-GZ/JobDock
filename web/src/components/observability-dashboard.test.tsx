// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ObservabilityDashboard } from "./observability-dashboard";
import { TooltipProvider } from "@/components/ui/tooltip";

describe("ObservabilityDashboard",()=>{
  afterEach(cleanup);
  it("adds and removes catalog widgets while preserving a useful empty state",async()=>{
    const user=userEvent.setup();
    render(<TooltipProvider><ObservabilityDashboard attemptID="attempt-1" ready numericSources={[]} matrices={[]} markers={[]} metricsDownloadURL="/metrics.csv" resourcesDownloadURL="/resources.csv"/></TooltipProvider>);
    expect(await screen.findByText("Build your metrics dashboard")).toBeTruthy();
    await user.click(screen.getAllByRole("button",{name:"Add widget"})[0]);
    await user.click(await screen.findByText("Bar plot"));
    await waitFor(()=>expect(document.querySelector('[data-widget-type="barplot"]')).toBeTruthy());
    expect(screen.getByText("No compatible numeric series has been reported yet.")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:"Remove widget"}));
    expect(await screen.findByText("Build your metrics dashboard")).toBeTruthy();
  });

  it("uses line, progress and matrix widgets as the backward-compatible default",async()=>{
    render(<TooltipProvider><ObservabilityDashboard attemptID="attempt-2" ready numericSources={[{kind:"metric",name:"loss",title:"loss",unit:"ratio",points:[{timestamp:1_000,value:.5}]}]} progress={{attempt_id:"attempt-2",simple:{value:.5},milestones:[],reached:[]}} matrices={[{id:1,attempt_id:"attempt-2",name:"validation",values:[[2]],labels:["cat"]}]} markers={[]} metricsDownloadURL="/metrics.csv" resourcesDownloadURL="/resources.csv"/></TooltipProvider>);
    await waitFor(()=>expect(document.querySelectorAll("[data-widget-id]")).toHaveLength(3));
    expect(document.querySelector('[data-widget-type="lineplot"]')).toBeTruthy();
    expect(document.querySelector('[data-widget-type="progress"]')).toBeTruthy();
    expect(document.querySelector('[data-widget-type="confusion_matrix"]')).toBeTruthy();
  });

  it("configures multiple compatible series and explicit scatter axes without fetching data",async()=>{
    const user=userEvent.setup(),sources=[
      {kind:"metric" as const,name:"loss",title:"loss",unit:"ratio",points:[{timestamp:1_000,value:.8,step:1},{timestamp:2_000,value:.4,step:2}]},
      {kind:"metric" as const,name:"duration",title:"duration",unit:"seconds",points:[{timestamp:1_000,value:8,step:1},{timestamp:2_000,value:5,step:2}]},
    ];
    render(<TooltipProvider><ObservabilityDashboard attemptID="attempt-3" ready numericSources={sources} matrices={[]} markers={[]} metricsDownloadURL="/metrics.csv" resourcesDownloadURL="/resources.csv"/></TooltipProvider>);
    await waitFor(()=>expect(document.querySelectorAll("[data-widget-id]")).toHaveLength(2));
    await user.click(screen.getAllByRole("button",{name:"Configure widget"})[0]);
    await user.click(screen.getByRole("checkbox",{name:"duration (seconds)"}));
    await user.click(screen.getByRole("button",{name:"Apply"}));
    expect(await screen.findByText("Different units use independent Y scales.")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:"Add widget"}));
    await user.click(await screen.findByText("Scatter plot"));
    expect(await screen.findByText("duration by loss")).toBeTruthy();
    await user.click(screen.getAllByRole("button",{name:"Configure widget"}).at(-1)!);
    expect(screen.getByRole("combobox",{name:"X series"})).toBeTruthy();
    expect(screen.getByRole("combobox",{name:"Y series"})).toBeTruthy();
  });

  it("offers accessible reorder, resize, and default-layout actions",async()=>{
    const user=userEvent.setup(),sources=[
      {kind:"metric" as const,name:"loss",title:"loss",unit:"ratio",points:[{timestamp:1_000,value:.8}]},
      {kind:"metric" as const,name:"accuracy",title:"accuracy",unit:"ratio",points:[{timestamp:1_000,value:.6}]},
    ];
    render(<TooltipProvider><ObservabilityDashboard attemptID="attempt-layout" ready numericSources={sources} matrices={[]} markers={[]} metricsDownloadURL="/metrics.csv" resourcesDownloadURL="/resources.csv"/></TooltipProvider>);
    await waitFor(()=>expect(document.querySelectorAll("[data-widget-id]")).toHaveLength(2));
    const initial=[...document.querySelectorAll<HTMLElement>("[data-widget-id]")].map(item=>item.dataset.widgetId);
    await user.click(screen.getAllByRole("button",{name:"Move or resize widget"})[1]);
    await user.click(await screen.findByText("Move earlier"));
    expect([...document.querySelectorAll<HTMLElement>("[data-widget-id]")].map(item=>item.dataset.widgetId)).toEqual(initial.reverse());
    await user.click(screen.getAllByRole("button",{name:"Move or resize widget"})[0]);
    await user.click(await screen.findByText("Use full width"));
    expect(document.querySelector<HTMLElement>("[data-widget-id]")?.dataset.size).toBe("2x1");
    await user.click(screen.getByRole("button",{name:"Restore default layout"}));
    expect(document.querySelector<HTMLElement>("[data-widget-id]")?.dataset.size).toBe("1x1");
  });
});
