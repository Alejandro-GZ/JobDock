// @vitest-environment jsdom

import { cleanup,render,screen } from "@testing-library/react";
import { afterEach,describe,expect,it } from "vitest";
import { ScalarSummaryWidget,scalarValue,thresholdState } from "@/components/scalar-summary-widget";
import type { DashboardWidget } from "@/lib/dashboard-widgets";
import type { NumericWidgetSource } from "@/components/observability-dashboard";

const source:NumericWidgetSource={kind:"metric",name:"accuracy",title:"Accuracy",unit:"%",points:[{timestamp:1,value:72},{timestamp:2,value:84}],summary:{last:84,min:72,max:84,avg:78}};
const widget=(type:"kpi"|"gauge",extra:Partial<DashboardWidget>={}):DashboardWidget=>({id:type,type,size:{columns:4,rows:3},position:{x:0,y:0},sources:[{kind:"metric",name:"accuracy"}],scalar_aggregation:"last",...extra});

afterEach(cleanup);
describe("Scalar summary widgets",()=>{
  it("selects explicit bounded aggregations and evaluates both threshold directions",()=>{expect(scalarValue(source.summary!,"avg")).toBe(78);expect(thresholdState(84,80,90)).toBe("warning");expect(thresholdState(40,50,30,"lower_is_worse")).toBe("warning");expect(thresholdState(20,50,30,"lower_is_worse")).toBe("critical")});
  it("renders a KPI label, unit, delta, and threshold state",()=>{render(<ScalarSummaryWidget source={source} widget={widget("kpi",{title:"Model quality",show_delta:true,warning_value:80,critical_value:90})}/>);expect(screen.getByLabelText("Model quality: 84 %")).toBeTruthy();expect(screen.getByText("Warning")).toBeTruthy();expect(screen.getByTitle("Change from the previous observation").textContent).toContain("+12")});
  it("renders Gauge and Bullet domains with targets and safe empty data",()=>{const configured={domain_min:0,domain_max:100,target_value:90,warning_value:80,critical_value:95};const {rerender}=render(<ScalarSummaryWidget source={source} widget={widget("gauge",configured)}/>);expect(screen.getByRole("meter",{name:"Accuracy: 84 %"})).toBeTruthy();rerender(<ScalarSummaryWidget source={source} widget={widget("gauge",{...configured,gauge_style:"bullet"})}/>);expect(screen.getByLabelText("Target")).toBeTruthy();rerender(<ScalarSummaryWidget source={{...source,points:[],summary:undefined}} widget={widget("kpi")}/>);expect(screen.getByRole("status").textContent).toContain("No scalar observations")});
});
