// @vitest-environment jsdom

import { cleanup,render,screen } from "@testing-library/react";
import { afterEach,describe,expect,it } from "vitest";
import { StarPlot,buildStarAxes } from "@/components/star-plot";
import type { PlotSeries } from "@/components/observation-plot";

const series:PlotSeries[]=[
  {id:"metric:accuracy",title:"Accuracy",unit:"%",points:[{timestamp:1,value:60},{timestamp:2,value:80}]},
  {id:"metric:latency",title:"Latency",unit:"ms",points:[{timestamp:1,value:40},{timestamp:2,value:20}]},
  {id:"resource:gpu",title:"GPU",unit:"%",points:[{timestamp:1,value:50},{timestamp:2,value:70}]},
  {id:"metric:waiting",title:"Waiting",unit:"score",points:[]},
];
afterEach(cleanup);

describe("StarPlot",()=>{
  it("normalizes independent units with explicit manual and fixed ranges",()=>{const axes=buildStarAxes(series.slice(0,3),"all",{schema_version:1,series:{"metric:accuracy":{normalization:"manual",min:0,max:100,label:"Quality"},"metric:latency":{normalization:"zero_to_one"}}});expect(axes[0]).toMatchObject({label:"Quality",minimum:0,maximum:100,normalized:.8,mode:"manual"});expect(axes[1]).toMatchObject({minimum:0,maximum:1,normalized:1,mode:"zero_to_one"});expect(axes[2].normalized).toBe(1)});
  it("renders accessible radial axes and reports partial data safely",()=>{render(<StarPlot title="Model profile" series={series} appearance={{schema_version:1,show_grid:true}}/>);expect(screen.getByRole("img",{name:"Model profile with 3 of 4 axes"})).toBeTruthy();expect(screen.getByRole("status").textContent).toContain("3/4 axes available");expect(screen.getByText("Accuracy")).toBeTruthy();expect(screen.getByText(/% · 0–80/)).toBeTruthy()});
});
