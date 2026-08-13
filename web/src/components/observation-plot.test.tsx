// @vitest-environment jsdom

import {cleanup,render,screen} from "@testing-library/react";
import {afterEach,describe,expect,it} from "vitest";
import {ObservationPlot} from "./observation-plot";

describe("ObservationPlot",()=>{
  afterEach(cleanup);
  const loss={id:"metric:loss",title:"loss",unit:"ratio",points:[{timestamp:1_000,value:.8,step:1},{timestamp:2_000,value:.4,step:2}]};
  const duration={id:"metric:duration",title:"duration",unit:"seconds",points:[{timestamp:1_000,value:8,step:1},{timestamp:2_000,value:5,step:2}]};

  it("keeps names and units visible and separates incompatible Y scales",()=>{
    render(<ObservationPlot type="lineplot" title="Training" series={[loss,duration]} xAxis="step"/>);
    expect(screen.getAllByText("loss · ratio").length).toBeGreaterThan(0);
    expect(screen.getAllByText("duration · seconds").length).toBeGreaterThan(0);
    expect(screen.getByRole("status").textContent).toContain("independent Y scales");
    expect(screen.getByRole("img",{name:/Training lineplot with 4 points/i})).toBeTruthy();
  });

  it("matches explicit scatter X and Y sources by step",()=>{
    render(<ObservationPlot type="scatterplot" title="duration by loss" series={[loss,duration]}/>);
    expect(screen.getByRole("img",{name:/duration by loss scatterplot with 2 points/i})).toBeTruthy();
    expect(screen.getAllByText("loss · ratio").length).toBeGreaterThan(0);
    expect(screen.getAllByText("duration · seconds").length).toBeGreaterThan(0);
  });
});
