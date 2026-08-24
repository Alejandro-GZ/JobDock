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
    expect(widgetCatalog.map(item=>item.type)).toEqual(["lineplot","loss_curve","learning_curve","anomaly_timeline","barplot","area_chart","stacked_bar","scatterplot","starplot","histogram","boxplot","violin","heatmap","correlation_heatmap","confusion_matrix","data_grid","roc_curve","precision_recall_curve","calibration_curve","prediction_vs_actual","residual_plot","feature_importance","shap_summary","partial_dependence","embedding_scatter","cluster_scatter","bubble_chart","parallel_coordinates","pie_chart","donut_chart","treemap","waterfall","progress","logs","kpi","gauge"]);
    expect(new Set(widgetCatalog.map(item=>item.category))).toEqual(new Set(["trends","relationships","summaries","diagnostics","operational"]));
    const original=defaultDashboardWidgets(sources),snapshot=structuredClone(original);
    const added=widgetCatalog.map((item,index)=>createDashboardWidget(item.type,sources,`new-${index}`));
    expect(added.find(widget=>widget.type==="loss_curve")?.sources[0]?.kind).toBe("metric");
    expect(added.find(widget=>widget.type==="feature_importance")?.sources).toEqual([]);
    expect(added.find(widget=>widget.type==="scatterplot")?.sources.map(source=>source.role)).toEqual(["x","y"]);
    expect(added.find(widget=>widget.type==="starplot")).toMatchObject({sources:[{kind:"metric",name:"loss"},{kind:"metric",name:"accuracy"},{kind:"resource",name:"cpu"}]});
    expect(added.find(widget=>widget.type==="kpi")).toMatchObject({scalar_aggregation:"last"});
    expect(added.find(widget=>widget.type==="gauge")).toMatchObject({gauge_style:"gauge",scalar_aggregation:"last"});
    const removed=removeDashboardWidget(original,original[1].id);
    expect(original).toEqual(snapshot);
    expect(removed).toHaveLength(original.length-1);
    expect(compatibleSourceKinds("lineplot")).toEqual(["metric","resource"]);
  });

  it("keeps confusion, generic heatmap, and correlation sources semantically separate",()=>{
    const typed={...sources,matrices:["confusion","attention","features"],matrix_types:{confusion:"confusion_matrix",attention:"heatmap",features:"correlation"}};
    expect(createDashboardWidget("confusion_matrix",typed,"confusion").sources[0]?.name).toBe("confusion");
    expect(createDashboardWidget("heatmap",typed,"heatmap").sources[0]?.name).toBe("attention");
    expect(createDashboardWidget("correlation_heatmap",typed,"correlation").sources[0]?.name).toBe("features");
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
    const restored=restoreDashboardWidgets([{id:"saved",type:"lineplot",title:"Custom loss",size:{columns:2,rows:1},position:{x:1,y:9},sources:[{kind:"metric",name:"missing"}],x_axis:"step",appearance:{schema_version:1,subtitle:"Validation",color_scheme:"cool",series:{"metric:missing":{label:"Objective",unit:"ratio",color:"#ABCDEF"},invalid:{color:"red"}},legend:"open",show_grid:false,x_axis:{label:"Step",scale:"log",range:"manual",min:1,max:100},y_axis:{range:"auto"},line_style:"dotted",line_width:3,show_points:true,point_size:4,opacity:.7,future_property:true}}]);
    expect(restored?.[0]).toMatchObject({id:"saved",title:"Custom loss",position:{x:0,y:0},sources:[{kind:"metric",name:"missing"}],x_axis:"step",appearance:{schema_version:1,subtitle:"Validation",color_scheme:"cool",series:{"metric:missing":{label:"Objective",unit:"ratio",color:"#abcdef"}},legend:"open",show_grid:false,x_axis:{label:"Step",scale:"log",range:"manual",min:1,max:100},y_axis:{range:"auto"},line_style:"dotted",line_width:3,show_points:true,point_size:4,opacity:.7}});
    expect(restored?.[0].size.columns).toBe(12);
    expect(restoreDashboardWidgets([{id:"future-style",type:"lineplot",size:{columns:1,rows:1},position:{x:0,y:0},sources:[],appearance:{schema_version:2,color_scheme:"future"}}])?.[0].appearance).toBeUndefined();
    expect(restoreDashboardWidgets([{id:"star",type:"starplot",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"metric",name:"loss"},{kind:"metric",name:"accuracy"},{kind:"resource",name:"cpu"}],appearance:{schema_version:1,series:{"metric:loss":{normalization:"manual",min:0,max:10},"metric:accuracy":{normalization:"zero_to_one"}}}}])?.[0].appearance?.series).toEqual({"metric:loss":{normalization:"manual",min:0,max:10},"metric:accuracy":{normalization:"zero_to_one"}});
    expect(restoreDashboardWidgets([{id:"area",type:"area_chart",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"metric",name:"loss"}],appearance:{schema_version:1,stack_mode:"stacked"}}])?.[0].appearance?.stack_mode).toBe("stacked");
    expect(restoreDashboardWidgets([{id:"pie",type:"pie_chart",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"table",name:"classes"}],category_limit:9,group_small_categories:false}])?.[0]).toMatchObject({type:"pie_chart",category_limit:9,group_small_categories:false});
    expect(restoreDashboardWidgets([{id:"parallel",type:"parallel_coordinates",size:{columns:6,rows:4},position:{x:0,y:0},sources:[{kind:"table",name:"records"}],normalize_axes:false}])?.[0]).toMatchObject({type:"parallel_coordinates",normalize_axes:false});
    expect(restoreDashboardWidgets([{id:"score",type:"kpi",title:"Validation score",size:{columns:3,rows:2},position:{x:0,y:0},sources:[{kind:"metric",name:"score"}],scalar_aggregation:"avg",target_value:.9,warning_value:.7,critical_value:.5,threshold_direction:"lower_is_worse",show_delta:true}])?.[0]).toMatchObject({type:"kpi",scalar_aggregation:"avg",target_value:.9,warning_value:.7,critical_value:.5,threshold_direction:"lower_is_worse",show_delta:true});
    expect(restoreDashboardWidgets([{id:"bad",type:"future-widget",size:{columns:1,rows:1},position:{x:0,y:0},sources:[]}])).toBeNull();
  });
});
