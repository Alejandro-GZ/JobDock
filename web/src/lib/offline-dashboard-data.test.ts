import { afterEach, describe, expect, it } from "vitest";
import { clearOfflineDashboardSnapshot, configureOfflineDashboardSnapshot, offlineDashboardLogFragments, offlineDashboardTable } from "@/lib/offline-dashboard-data";

const table={attempt_id:"attempt-1",name:"features",subtype:"feature_importance",columns:[{name:"feature",type:"string" as const},{name:"importance",type:"number" as const}],items:[{cursor:1,timestamp:"2026-08-29T10:00:00Z",values:{feature:"age",importance:-.7}},{cursor:2,timestamp:"2026-08-29T10:00:01Z",values:{feature:"income",importance:.9}},{cursor:3,timestamp:"2026-08-29T10:00:02Z",values:{feature:"region",importance:.2}}],total:3};

describe("offline dashboard data",()=>{
  afterEach(clearOfflineDashboardSnapshot);

  it("serves filtered, sorted and paginated embedded table rows",()=>{
    configureOfflineDashboardSnapshot({attemptID:"attempt-1",tables:{features:table},logFragments:[]});
    const page=offlineDashboardTable("attempt-1","features","filter=feature%3Di&sort=importance&absolute=true&order=desc&limit=1");
    expect(page?.total).toBe(2);
    expect(page?.items.map(item=>item.values.feature)).toEqual(["income"]);
    expect(page?.next_cursor).toBe(2);
  });

  it("preserves ordered stream fragments and isolates attempts",()=>{
    configureOfflineDashboardSnapshot({attemptID:"attempt-1",tables:{},logFragments:[{stream:"stdout",text:"one\n"},{stream:"stderr",text:"two\n"},{stream:"stdout",text:"three\n"}]});
    expect(offlineDashboardLogFragments("attempt-1",["stdout","stderr"])?.map(item=>item.text).join("")).toBe("one\ntwo\nthree\n");
    expect(offlineDashboardLogFragments("attempt-1",["stderr"])).toEqual([{stream:"stderr",text:"two\n"}]);
    expect(offlineDashboardLogFragments("another-attempt",["stdout"])).toBeUndefined();
  });
});
