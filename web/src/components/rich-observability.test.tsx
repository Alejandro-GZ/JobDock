// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ConfusionMatrixWidget } from "./confusion-matrix-widget";
import { ProgressWidget } from "./progress-widget";

describe("rich observability widgets", () => {
  afterEach(cleanup);
  it("shows global and current milestone progress with upcoming stages", () => {
    render(<ProgressWidget state={{attempt_id:"attempt-1",global_progress:.6,milestones:[{name:"prepare",weight:.2},{name:"train",weight:.8},{name:"evaluate"}],reached:["prepare"],current:{value:.5,milestone:"train"}}}/>);
    expect(screen.getByText("60%")).toBeTruthy();
    expect(screen.getByText("50%")).toBeTruthy();
    expect(screen.getByText("prepare")).toBeTruthy();
    expect(screen.getByText("evaluate")).toBeTruthy();
  });
  it("renders a configured three-color progress gradient",()=>{
    render(<ProgressWidget state={{attempt_id:"attempt-1",global_progress:.6,milestones:[],reached:[]}} appearance={{schema_version:1,gradient:[{offset:0,color:"#16a34a"},{offset:.5,color:"#f59e0b"},{offset:1,color:"#dc2626"}]}}/>);expect((screen.getByLabelText("Global progress").firstElementChild as HTMLElement).style.background).toContain("linear-gradient");
  });

  it("switches a confusion matrix between absolute and row-normalized values", async () => {
    const user = userEvent.setup();
    render(<ConfusionMatrixWidget matrix={{id:1,attempt_id:"attempt-1",name:"validation",values:[[8,2],[1,9]],labels:["cat","dog"],step:5}}/>);
    const grid=screen.getByRole("grid",{name:"validation confusion matrix"});
    expect(Number(grid.getAttribute("data-cell-size"))).toBeGreaterThan(80);
    expect(Number.parseFloat(grid.style.width)).toBeLessThanOrEqual(310);
    expect(screen.getByTitle("cat → dog: 2")).toBeTruthy();
    await user.click(screen.getByRole("button", {name:"Normalized"}));
    expect(screen.getByTitle("cat → dog: 20.00%")).toBeTruthy();
  });
  it("uses a template-provided initial matrix presentation without locking the control",async()=>{
    const user=userEvent.setup();
    render(<ConfusionMatrixWidget initialMode="normalized" matrix={{id:2,attempt_id:"attempt-1",name:"validation",values:[[8,2],[1,9]],labels:["cat","dog"]}}/>);
    expect(screen.getByTitle("cat → dog: 20.00%")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:"Absolute"}));
    expect(screen.getByTitle("cat → dog: 2")).toBeTruthy();
  });
});
