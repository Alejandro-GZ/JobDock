// @vitest-environment jsdom
import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {render,screen,waitFor} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {api} from "@/api";
import {DataGridWidget} from "@/components/data-grid-widget";
import {EvaluationCurveWidget} from "@/components/evaluation-curve-widget";
import {ProjectionScatterWidget} from "@/components/projection-scatter-widget";
import {RegressionDiagnosticsWidget} from "@/components/regression-diagnostics-widget";
import {TabularChartWidget} from "@/components/tabular-chart-widget";
import type {DashboardWidget} from "@/lib/dashboard-widgets";
import type {TablePage} from "@/types";

vi.mock("@/api",()=>({api:{table:vi.fn()}}));

const page:TablePage={
  attempt_id:"attempt",
  name:"observations",
  subtype:"table",
  columns:[{name:"label",type:"string"},{name:"x",type:"number"},{name:"y",type:"number"},{name:"size",type:"number"}],
  items:[{cursor:1,timestamp:"2026-08-24T12:00:00Z",values:{label:"candidate",x:.2,y:.8,size:4}}],
  total:1,
};

function view(component:React.ReactNode){
  const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
  return render(<QueryClientProvider client={client}>{component}</QueryClientProvider>);
}

describe("tabular observability widgets",()=>{
  beforeEach(()=>vi.clearAllMocks());
  it("renders a bounded data grid with typed columns",async()=>{
    vi.mocked(api.table).mockResolvedValueOnce(page);
    const widget:DashboardWidget={id:"grid",type:"data_grid",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"table",name:"observations"}]};
    view(<DataGridWidget jobID="job" attemptID="attempt" widget={widget} onUpdate={()=>{}}/>);
    expect(await screen.findByLabelText("observations data grid")).toBeTruthy();
    expect(screen.getByText("candidate")).toBeTruthy();
    expect(api.table).toHaveBeenCalledWith("job","attempt","observations",expect.stringContaining("limit=50"));
  });

  it("renders reported ROC points and the perfect-reference line",async()=>{
    vi.mocked(api.table).mockResolvedValueOnce({...page,name:"roc",subtype:"roc",columns:[{name:"fpr",type:"number"},{name:"tpr",type:"number"},{name:"threshold",type:"number",nullable:true}],items:[{cursor:1,timestamp:"2026-08-24T12:00:00Z",values:{fpr:.2,tpr:.8,threshold:.6}}]});
    const widget:DashboardWidget={id:"roc",type:"roc_curve",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"table",name:"roc"}]};
    view(<EvaluationCurveWidget jobID="job" attemptID="attempt" widget={widget}/>);
    expect(await screen.findByLabelText("ROC curve")).toBeTruthy();
    expect(screen.getByText("False-positive rate")).toBeTruthy();
  });

  it("renders bubble dimensions from one bounded table response",async()=>{
    vi.mocked(api.table).mockResolvedValueOnce(page);
    const widget:DashboardWidget={id:"bubble",type:"bubble_chart",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"table",name:"observations"}],table_columns:["x","y","size","label"]};
    view(<TabularChartWidget jobID="job" attemptID="attempt" widget={widget} onUpdate={()=>{}}/>);
    expect(await screen.findByLabelText("Bubble Chart")).toBeTruthy();
    expect(screen.getByLabelText("Color or group")).toBeTruthy();
    expect(api.table).toHaveBeenCalledTimes(1);
  });

  it("keeps embedding scatter geometry square inside a wide tile",async()=>{
    vi.mocked(api.table).mockResolvedValueOnce({...page,subtype:"projection",items:[{cursor:1,timestamp:"2026-08-24T12:00:00Z",values:{sample_id:"a",x:0,y:0,label:"A"}},{cursor:2,timestamp:"2026-08-24T12:00:01Z",values:{sample_id:"b",x:1,y:1,label:"B"}}]});
    const bounds=vi.spyOn(HTMLElement.prototype,"getBoundingClientRect").mockReturnValue({width:900,height:240,top:0,left:0,right:900,bottom:240,x:0,y:0,toJSON:()=>({})});
    vi.stubGlobal("ResizeObserver",class{constructor(private callback:ResizeObserverCallback){}observe(){this.callback([],this as unknown as ResizeObserver)}disconnect(){}unobserve(){}});
    const widget:DashboardWidget={id:"projection",type:"embedding_scatter",size:{columns:12,rows:3},position:{x:0,y:0},sources:[{kind:"table",name:"observations"}]};
    view(<ProjectionScatterWidget jobID="job" attemptID="attempt" widget={widget} onUpdate={()=>{}}/>);
    const plot=await screen.findByRole("img",{name:"Embedding scatter with 2 points"});
    await waitFor(()=>expect(plot.getAttribute("width")).toBe("900"));
    expect(plot.getAttribute("height")).toBe("240");expect(plot.getAttribute("data-plot-size")).toBe("164x164");expect(plot.hasAttribute("preserveAspectRatio")).toBe(false);
    bounds.mockRestore();vi.unstubAllGlobals();
  });

  it("preserves equal axes for prediction scatter in a wide tile",async()=>{
    vi.mocked(api.table).mockResolvedValueOnce({...page,subtype:"regression_diagnostics",columns:[{name:"actual",type:"number",unit:"score"},{name:"prediction",type:"number",unit:"score"}],items:[{cursor:1,timestamp:"2026-08-24T12:00:00Z",values:{actual:0,prediction:.1}},{cursor:2,timestamp:"2026-08-24T12:00:01Z",values:{actual:1,prediction:.9}}]});
    const bounds=vi.spyOn(HTMLElement.prototype,"getBoundingClientRect").mockReturnValue({width:900,height:240,top:0,left:0,right:900,bottom:240,x:0,y:0,toJSON:()=>({})});
    vi.stubGlobal("ResizeObserver",class{constructor(private callback:ResizeObserverCallback){}observe(){this.callback([],this as unknown as ResizeObserver)}disconnect(){}unobserve(){}});
    const widget:DashboardWidget={id:"regression",type:"prediction_vs_actual",size:{columns:12,rows:3},position:{x:0,y:0},sources:[{kind:"table",name:"observations"}]};
    view(<RegressionDiagnosticsWidget jobID="job" attemptID="attempt" widget={widget}/>);
    const plot=await screen.findByRole("img",{name:"Prediction versus actual scatter with 2 points"});
    await waitFor(()=>expect(plot.getAttribute("width")).toBe("900"));
    expect(plot.getAttribute("height")).toBe("240");expect(plot.getAttribute("data-plot-size")).toBe("168x168");expect(plot.hasAttribute("preserveAspectRatio")).toBe(false);
    bounds.mockRestore();vi.unstubAllGlobals();
  });
});
