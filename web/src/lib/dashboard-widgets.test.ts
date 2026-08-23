import { describe, expect, it } from "vitest";
import { compatibleSourceKinds, createDashboardWidget, defaultDashboardWidgets, layoutDashboardWidgets, moveDashboardWidget, removeDashboardWidget, resizeDashboardWidget, restoreDashboardWidgets, widgetCatalog } from "./dashboard-widgets";

const sources={metrics:["loss","accuracy"],resources:["cpu"],matrices:["validation"],progress:true,logs:true};

describe("dashboard widget model",()=>{
  it("builds a default dashboard equivalent to all currently available observations",()=>{
    const widgets=defaultDashboardWidgets(sources);
    expect(widgets.map(widget=>widget.type)).toEqual(["progress","lineplot","lineplot","lineplot","confusion_matrix"]);
    expect(widgets.flatMap(widget=>widget.sources.map(source=>`${source.kind}:${source.name}`))).toEqual(["progress:progress","metric:loss","metric:accuracy","resource:cpu","matrix:validation"]);
    expect(widgets.every(widget=>widget.id&&widget.size&&widget.position)).toBe(true);
    expect(widgets[0].position).toEqual({x:0,y:0});
    expect(widgets[1].position).toEqual({x:0,y:3});
  });

  it("supports the complete catalog without mutating telemetry or existing widgets",()=>{
    expect(widgetCatalog.map(item=>item.type)).toEqual(["lineplot","barplot","scatterplot","confusion_matrix","progress","logs","gauge"]);
    const original=defaultDashboardWidgets(sources),snapshot=structuredClone(original);
    const added=widgetCatalog.map((item,index)=>createDashboardWidget(item.type,sources,`new-${index}`));
    expect(added.map(widget=>widget.sources[0]?.kind)).toEqual(["metric","metric","metric","matrix","progress","log","metric"]);
    expect(added[2].sources.map(source=>source.role)).toEqual(["x","y"]);
    expect(added[6]).toMatchObject({type:"gauge",gauge_max_mode:"historical"});
    const removed=removeDashboardWidget(original,original[1].id);
    expect(original).toEqual(snapshot);
    expect(removed).toHaveLength(original.length-1);
    expect(compatibleSourceKinds("lineplot")).toEqual(["metric","resource"]);
  });

  it("reorders and resizes without overlapping occupied grid cells",()=>{
    const widgets=layoutDashboardWidgets([
      {id:"one",type:"lineplot",size:{columns:1,rows:2},position:{x:0,y:0},sources:[]},
      {id:"two",type:"barplot",size:{columns:1,rows:1},position:{x:0,y:0},sources:[]},
      {id:"three",type:"progress",size:{columns:2,rows:1},position:{x:0,y:0},sources:[]},
    ]);
    expect(widgets.map(item=>item.position)).toEqual([{x:0,y:0},{x:1,y:0},{x:2,y:0}]);
    const moved=moveDashboardWidget(widgets,"three","earlier");
    expect(moved.map(item=>item.id)).toEqual(["one","three","two"]);
    const resized=resizeDashboardWidget(moved,"two",{columns:2,rows:2});
    expect(resized.find(item=>item.id==="two")?.size).toEqual({columns:2,rows:2});
  });

  it("restores versioned widget data and safely rejects invalid saved layouts",()=>{
    const restored=restoreDashboardWidgets([{id:"saved",type:"lineplot",title:"Custom loss",size:{columns:2,rows:1},position:{x:1,y:9},sources:[{kind:"metric",name:"missing"}],x_axis:"step",appearance:{schema_version:1,color_scheme:"cool",legend:"open",line_style:"dashed",show_points:true,future_property:true}}]);
    expect(restored?.[0]).toMatchObject({id:"saved",title:"Custom loss",position:{x:0,y:0},sources:[{kind:"metric",name:"missing"}],x_axis:"step",appearance:{schema_version:1,color_scheme:"cool",legend:"open",line_style:"dashed",show_points:true}});
    expect(restored?.[0].size.columns).toBe(12);
    expect(restoreDashboardWidgets([{id:"future-style",type:"lineplot",size:{columns:1,rows:1},position:{x:0,y:0},sources:[],appearance:{schema_version:2,color_scheme:"future"}}])?.[0].appearance).toBeUndefined();
    expect(restoreDashboardWidgets([{id:"bad",type:"future-widget",size:{columns:1,rows:1},position:{x:0,y:0},sources:[]}])).toBeNull();
  });
});
