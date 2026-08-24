// @vitest-environment jsdom
import {render,screen} from "@testing-library/react";
import {describe,expect,it} from "vitest";
import {HeatmapWidget} from "./heatmap-widget";
import type {MatrixObservation} from "@/types";

const matrix:MatrixObservation={id:1,attempt_id:"attempt",name:"attention",matrix_type:"heatmap",values:[[0,null],[.5,1]],row_labels:["query-a","query-b"],column_labels:["key-a","key-b"],unit:"score"};

describe("HeatmapWidget",()=>{
  it("renders independent axes, null cells, and cell tooltips",()=>{
    const {container}=render(<HeatmapWidget matrix={matrix}/>);
    expect(screen.getByRole("img",{name:"attention heatmap with 2 rows and 2 columns"})).toBeTruthy();
    expect(container.querySelectorAll('rect[data-null="true"]')).toHaveLength(1);
    expect(container.querySelector("title")?.textContent).toContain("query-a × key-a: 0 score");
  });

  it("uses the full correlation domain unless a manual domain is configured",()=>{
    const correlation={...matrix,name:"features",matrix_type:"correlation" as const,values:[[1,-.25],[-.25,1]],row_labels:["a","b"],column_labels:["a","b"]};
    const {container,rerender}=render(<HeatmapWidget matrix={correlation} correlation/>);
    expect(container.textContent).toContain("-1");
    rerender(<HeatmapWidget matrix={correlation} correlation appearance={{schema_version:1,heatmap_scale:"manual",heatmap_min:-.5,heatmap_max:.5,heatmap_palette:"diverging"}}/>);
    expect(container.textContent).toMatch(/-0[.,]5/);
  });
});
