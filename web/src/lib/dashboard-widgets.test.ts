import { describe, expect, it } from "vitest";
import { compatibleSourceKinds, createDashboardWidget, defaultDashboardWidgets, removeDashboardWidget, widgetCatalog } from "./dashboard-widgets";

const sources={metrics:["loss","accuracy"],resources:["cpu"],matrices:["validation"],progress:true};

describe("dashboard widget model",()=>{
  it("builds a default dashboard equivalent to all currently available observations",()=>{
    const widgets=defaultDashboardWidgets(sources);
    expect(widgets.map(widget=>widget.type)).toEqual(["progress","lineplot","lineplot","lineplot","confusion_matrix"]);
    expect(widgets.flatMap(widget=>widget.sources.map(source=>`${source.kind}:${source.name}`))).toEqual(["progress:progress","metric:loss","metric:accuracy","resource:cpu","matrix:validation"]);
    expect(widgets.every(widget=>widget.id&&widget.size&&widget.position)).toBe(true);
    expect(widgets[0].position).toEqual({x:0,y:0});
    expect(widgets[1].position).toEqual({x:0,y:1});
  });

  it("supports the complete catalog without mutating telemetry or existing widgets",()=>{
    expect(widgetCatalog.map(item=>item.type)).toEqual(["lineplot","barplot","scatterplot","confusion_matrix","progress"]);
    const original=defaultDashboardWidgets(sources),snapshot=structuredClone(original);
    const added=widgetCatalog.map((item,index)=>createDashboardWidget(item.type,sources,`new-${index}`));
    expect(added.map(widget=>widget.sources[0]?.kind)).toEqual(["metric","metric","metric","matrix","progress"]);
    expect(added[2].sources.map(source=>source.role)).toEqual(["x","y"]);
    const removed=removeDashboardWidget(original,original[1].id);
    expect(original).toEqual(snapshot);
    expect(removed).toHaveLength(original.length-1);
    expect(compatibleSourceKinds("lineplot")).toEqual(["metric","resource"]);
  });
});
