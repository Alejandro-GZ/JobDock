// @vitest-environment jsdom

import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,describe,expect,it,vi} from "vitest";
import {ObservationPlot} from "./observation-plot";

describe("ObservationPlot",()=>{
  afterEach(()=>{cleanup();vi.restoreAllMocks();vi.unstubAllGlobals()});
  const loss={id:"metric:loss",title:"loss",unit:"ratio",points:[{timestamp:1_000,value:.8,step:1},{timestamp:2_000,value:.4,step:2}]};
  const duration={id:"metric:duration",title:"duration",unit:"seconds",points:[{timestamp:1_000,value:8,step:1},{timestamp:2_000,value:5,step:2}]};

  it("opens a compact legend and separates incompatible Y scales",async()=>{
    const user=userEvent.setup();
    render(<ObservationPlot type="lineplot" title="Training" series={[loss,duration]} xAxis="step"/>);
    expect(screen.getByText("Training").className).toContain("group-hover/widget:opacity-0");
    expect(screen.queryByText("loss")).toBeNull();
    await user.click(screen.getByRole("button",{name:"Training legend"}));
    expect(screen.getByText("loss")).toBeTruthy();
    expect(screen.getByText("duration")).toBeTruthy();
    expect(screen.getByText(/independent Y scales/i)).toBeTruthy();
    expect(screen.getByRole("img",{name:/Training lineplot with 4 points/i})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Zoom in Training"})).toBeTruthy();
    expect(document.querySelector('[data-y-axis="ratio"]')).toBeTruthy();
    expect(document.querySelector('[data-y-axis="seconds"]')).toBeTruthy();
    expect(screen.getByText("Step")).toBeTruthy();
  });

  it("matches explicit scatter X and Y sources by step and fills its tile",async()=>{
    const user=userEvent.setup();
    vi.spyOn(HTMLElement.prototype,"getBoundingClientRect").mockReturnValue({width:360,height:180,top:0,left:0,right:360,bottom:180,x:0,y:0,toJSON:()=>({})});
    vi.stubGlobal("ResizeObserver",class{constructor(private callback:ResizeObserverCallback){}observe(){this.callback([],this as unknown as ResizeObserver)}disconnect(){}unobserve(){}});
    render(<ObservationPlot type="scatterplot" title="duration by loss" series={[loss,duration]}/>);
    const plot=screen.getByRole("img",{name:/duration by loss scatterplot with 2 points/i});
    await waitFor(()=>expect(plot.getAttribute("width")).toBe("360"));
    expect(plot.getAttribute("height")).toBe("180");
    expect(plot.hasAttribute("preserveAspectRatio")).toBe(false);
    expect(screen.getByRole("button",{name:"Zoom in duration by loss"})).toBeTruthy();
    await user.click(screen.getByRole("button",{name:"duration by loss legend"}));
    expect(screen.getByText("loss")).toBeTruthy();
    expect(screen.getByText("duration")).toBeTruthy();
  });

  it("keeps the shared toolbar inside the chart and restores the exact initial domain",async()=>{
    const user=userEvent.setup();
    render(<ObservationPlot type="barplot" title="Training" series={[loss]} xAxis="step"/>);
    const plot=screen.getByRole("img",{name:/Training barplot with 2 points/i}),initial=plot.getAttribute("data-domain"),toolbar=document.querySelector<HTMLElement>("[data-plot-toolbar]");
    expect(toolbar?.className).toContain("bottom-1");
    expect(toolbar?.className).toContain("left-1/2");
    expect(toolbar?.className).toContain("opacity-0");
    expect(toolbar?.className).toContain("group-hover:opacity-100");
    await user.click(screen.getByRole("button",{name:"Zoom in Training"}));
    expect(plot.getAttribute("data-domain")).not.toBe(initial);
    await user.click(screen.getByRole("button",{name:"Reset zoom Training"}));
    expect(plot.getAttribute("data-domain")).toBe(initial);
    expect(screen.getByRole("button",{name:"Training legend"}).getAttribute("aria-expanded")).toBe("false");
  });
  it("renders materialized presentation settings without chart-library props",()=>{
    const {container}=render(<ObservationPlot type="lineplot" title="Styled" series={[loss]} xAxis="step" appearance={{schema_version:1,color_scheme:"warm",legend:"hidden",line_style:"dashed",show_points:true}}/>);
    expect(screen.queryByRole("button",{name:"Styled legend"})).toBeNull();
    expect(container.querySelector('path[stroke-dasharray="6 4"]')).toBeTruthy();
    expect(container.querySelectorAll('[role="img"] circle')).toHaveLength(2);
    expect(container.querySelector('path[stroke-dasharray="6 4"]')?.getAttribute("stroke")).toBe("#ea580c");
  });
  it("applies common axes, series, grid, marker and opacity settings",()=>{
    const {container}=render(<ObservationPlot type="lineplot" title="Styled" series={[loss]} xAxis="step" appearance={{schema_version:1,subtitle:"Validation",series:{"metric:loss":{label:"Objective",unit:"score",color:"#123abc"}},show_grid:false,x_axis:{label:"Epoch",scale:"log",range:"manual",min:1,max:10},y_axis:{label:"Loss",unit:"ratio",range:"manual",min:0,max:1},line_style:"dotted",line_width:3,show_points:true,point_size:5,opacity:.6}}/>);
    expect(screen.getByText("Validation")).toBeTruthy();expect(screen.getByText("Epoch")).toBeTruthy();expect(screen.getByText("Loss (ratio)")).toBeTruthy();
    const path=container.querySelector('path[stroke="#123abc"]')!;expect(path.getAttribute("stroke-width")).toBe("3");expect(path.getAttribute("stroke-dasharray")).toBe("2 3");expect(path.parentElement?.getAttribute("opacity")).toBe("0.6");expect(container.querySelectorAll('circle[r="5"]')).toHaveLength(2);expect(container.querySelectorAll("line.stroke-border")).toHaveLength(0);
    expect(screen.getByRole("button",{name:"Styled legend"})).toBeTruthy();
  });
  it("renders overlapping areas with explicit gaps for misaligned categories",()=>{
    const shifted={...duration,unit:"ratio",points:[{timestamp:1_000,value:.2,step:1},{timestamp:3_000,value:.7,step:3}]};
    const {container}=render(<ObservationPlot type="area_chart" title="Quality" series={[loss,shifted]} xAxis="step" appearance={{schema_version:1,stack_mode:"overlap"}}/>);
    const plot=screen.getByRole("img",{name:/Quality area_chart with 4 points/i});
    expect(plot.getAttribute("data-missing-values")).toBe("gap");
    expect(container.querySelectorAll('path[fill-opacity]')).toHaveLength(3);
    expect(screen.getByRole("button",{name:"Zoom in Quality"})).toBeTruthy();
  });
  it("stacks shared bar categories deterministically and treats missing components as zero",()=>{
    const shifted={...duration,unit:"ratio",points:[{timestamp:1_000,value:.2,step:1},{timestamp:3_000,value:.7,step:3}]};
    const {container}=render(<ObservationPlot type="stacked_bar" title="Composition" series={[loss,shifted]} xAxis="step"/>);
    const plot=screen.getByRole("img",{name:/Composition stacked_bar with 4 points/i});
    expect(plot.getAttribute("data-missing-values")).toBe("zero");
    expect(container.querySelectorAll("[data-bar-mark]")).toHaveLength(6);
    fireEvent.pointerMove(plot,{clientX:720,clientY:100});
    expect(screen.getByText(/loss: 0 \(missing\)/i)).toBeTruthy();
  });
  it.each(["barplot","stacked_bar"] as const)("keeps the first and last %s marks inside the vertical axes",async type=>{
    vi.spyOn(HTMLElement.prototype,"getBoundingClientRect").mockReturnValue({width:360,height:180,top:0,left:0,right:360,bottom:180,x:0,y:0,toJSON:()=>({})});
    vi.stubGlobal("ResizeObserver",class{constructor(private callback:ResizeObserverCallback){}observe(){this.callback([],this as unknown as ResizeObserver)}disconnect(){}unobserve(){}});
    const second={...duration,unit:"ratio"};
    const {container}=render(<ObservationPlot type={type} title="Bounded bars" series={[loss,second]} xAxis="step"/>);
    const plot=screen.getByRole("img",{name:new RegExp(`Bounded bars ${type}`)});
    await waitFor(()=>expect(plot.getAttribute("width")).toBe("360"));
    const marks=[...container.querySelectorAll<SVGRectElement>("[data-bar-mark]")];
    expect(marks.length).toBeGreaterThan(0);
    for(const mark of marks){const x=Number(mark.getAttribute("x")),width=Number(mark.getAttribute("width"));expect(x).toBeGreaterThanOrEqual(58);expect(x+width).toBeLessThanOrEqual(342)}
    expect(container.querySelector("[data-plot-marks]")?.getAttribute("clip-path")).toMatch(/^url\(#.+\)$/);
  });
  it("rejects stacked series with incompatible units",()=>{
    render(<ObservationPlot type="stacked_bar" series={[loss,duration]} xAxis="step"/>);
    expect(screen.getByText("Compatible units required")).toBeTruthy();
  });
});
