// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ConfusionMatrixWidget } from "./confusion-matrix-widget";
import { ProgressWidget } from "./progress-widget";

describe("rich observability widgets", () => {
  it("shows global and current milestone progress with upcoming stages", () => {
    render(<ProgressWidget state={{attempt_id:"attempt-1",global_progress:.6,milestones:[{name:"prepare",weight:.2},{name:"train",weight:.8},{name:"evaluate"}],reached:["prepare"],current:{value:.5,milestone:"train"}}}/>);
    expect(screen.getByText("60%")).toBeTruthy();
    expect(screen.getByText("50%")).toBeTruthy();
    expect(screen.getByText("prepare")).toBeTruthy();
    expect(screen.getByText("evaluate")).toBeTruthy();
  });

  it("switches a confusion matrix between absolute and row-normalized values", async () => {
    const user = userEvent.setup();
    render(<ConfusionMatrixWidget matrix={{id:1,attempt_id:"attempt-1",name:"validation",values:[[8,2],[1,9]],labels:["cat","dog"],step:5}}/>);
    expect(screen.getByTitle("cat → dog: 2")).toBeTruthy();
    await user.click(screen.getByRole("button", {name:"Normalized"}));
    expect(screen.getByTitle("cat → dog: 20.00%")).toBeTruthy();
  });
});
