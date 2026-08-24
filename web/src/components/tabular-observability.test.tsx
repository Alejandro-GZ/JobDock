// @vitest-environment jsdom
import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {render,screen} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {api} from "@/api";
import {DataGridWidget} from "@/components/data-grid-widget";
import {EvaluationCurveWidget} from "@/components/evaluation-curve-widget";
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
});
