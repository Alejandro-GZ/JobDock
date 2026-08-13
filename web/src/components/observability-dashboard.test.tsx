// @vitest-environment jsdom

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ObservabilityDashboard } from "./observability-dashboard";
import { TooltipProvider } from "@/components/ui/tooltip";

describe("ObservabilityDashboard",()=>{
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
    render(<TooltipProvider><ObservabilityDashboard attemptID="attempt-2" ready numericSources={[{kind:"metric",name:"loss",title:"loss",points:[{timestamp:1_000,value:.5}]}]} progress={{attempt_id:"attempt-2",simple:{value:.5},milestones:[],reached:[]}} matrices={[{id:1,attempt_id:"attempt-2",name:"validation",values:[[2]],labels:["cat"]}]} markers={[]} metricsDownloadURL="/metrics.csv" resourcesDownloadURL="/resources.csv"/></TooltipProvider>);
    await waitFor(()=>expect(document.querySelectorAll("[data-widget-id]")).toHaveLength(3));
    expect(document.querySelector('[data-widget-type="lineplot"]')).toBeTruthy();
    expect(document.querySelector('[data-widget-type="progress"]')).toBeTruthy();
    expect(document.querySelector('[data-widget-type="confusion_matrix"]')).toBeTruthy();
  });
});
