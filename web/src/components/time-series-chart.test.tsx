// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TimeSeriesChart } from "./time-series-chart";

describe("TimeSeriesChart", () => {
  it("renders accessible statistics, points, tooltip controls, and zoom actions", async () => {
    const user = userEvent.setup();
    render(<TimeSeriesChart title="loss" points={[{timestamp:1_000,value:4,step:1},{timestamp:2_000,value:2,step:2}]} summary={{last:2,min:2,max:4}}/>);
    expect(screen.getByRole("img", {name:/loss time series with 2 points/i})).toBeTruthy();
    expect(screen.getByText("last 2")).toBeTruthy();
    expect(screen.getByText("min 2")).toBeTruthy();
    expect(screen.getByText("max 4")).toBeTruthy();
    await user.click(screen.getByRole("button", {name:"Zoom in loss"}));
    await user.click(screen.getByRole("button", {name:"Zoom out loss"}));
    await user.click(screen.getByRole("button", {name:"Reset zoom loss"}));
  });
});
