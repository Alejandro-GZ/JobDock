// @vitest-environment jsdom

import {cleanup,render,screen,waitFor} from "@testing-library/react";
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
    expect(screen.queryByText("loss")).toBeNull();
    await user.click(screen.getByRole("button",{name:"Training legend"}));
    expect(screen.getByText("loss")).toBeTruthy();
    expect(screen.getByText("duration")).toBeTruthy();
    expect(screen.getByText(/independent Y scales/i)).toBeTruthy();
    expect(screen.getByRole("img",{name:/Training lineplot with 4 points/i})).toBeTruthy();
    expect(screen.getByRole("button",{name:"Zoom in Training"})).toBeTruthy();
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
});
