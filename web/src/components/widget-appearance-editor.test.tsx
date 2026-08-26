// @vitest-environment jsdom

import { useState } from "react";
import { cleanup,fireEvent,render,screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach,beforeAll,describe,expect,it } from "vitest";
import { WidgetAppearanceEditor } from "@/components/widget-appearance-editor";
import type { DashboardWidget } from "@/lib/dashboard-widgets";

afterEach(cleanup);

beforeAll(() => {
  Object.defineProperties(HTMLElement.prototype, {
    hasPointerCapture: { configurable: true, value: () => false },
    setPointerCapture: { configurable: true, value: () => undefined },
    scrollIntoView: { configurable: true, value: () => undefined },
  });
});

function Harness(){const[draft,setDraft]=useState<DashboardWidget>({id:"star",type:"starplot",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"metric",name:"accuracy"},{kind:"metric",name:"latency"},{kind:"resource",name:"gpu"}]});return <><WidgetAppearanceEditor draft={draft} sources={[{key:"metric:accuracy",label:"Accuracy",unit:"%"},{key:"metric:latency",label:"Latency",unit:"ms"},{key:"resource:gpu",label:"GPU",unit:"%"}]} onChange={setDraft}/><output>{JSON.stringify(draft.appearance)}</output></>}
function LogsHarness(){const[draft,setDraft]=useState<DashboardWidget>({id:"logs",type:"logs",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"log",name:"stdout"},{kind:"log",name:"stderr"}]});return <><WidgetAppearanceEditor draft={draft} sources={[{key:"log:stdout",label:"stdout"},{key:"log:stderr",label:"stderr"}]} onChange={setDraft}/><output>{JSON.stringify(draft.appearance)}</output></>}

describe("WidgetAppearanceEditor STAR axes",()=>{
  it("configures a manual independent range without exposing Cartesian axes",async()=>{render(<Harness/>);expect(screen.getByText("Radial axes, units and ranges")).toBeTruthy();expect(screen.queryByLabelText("X axis label")).toBeNull();await userEvent.click(screen.getByRole("combobox",{name:"Accuracy normalization"}));await userEvent.click(screen.getByRole("option",{name:"Manual limits"}));const minimum=screen.getByLabelText("Accuracy minimum"),maximum=screen.getByLabelText("Accuracy maximum");fireEvent.change(minimum,{target:{value:"10"}});fireEvent.change(maximum,{target:{value:"90"}});expect(screen.getByText(/"normalization":"manual"/).textContent).toContain('"min":10');expect(screen.getByText(/"normalization":"manual"/).textContent).toContain('"max":90')});
});

describe("WidgetAppearanceEditor logs",()=>{
  it("offers an explicit console background control",()=>{render(<LogsHarness/>);const background=screen.getByLabelText("Console background color");expect(background).toHaveProperty("value","#09090b");fireEvent.change(background,{target:{value:"#112233"}});expect(screen.getByText(/"background_color":"#112233"/)).toBeTruthy();expect(screen.queryByLabelText("Background color")).toBeNull()});
});
