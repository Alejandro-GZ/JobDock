import {describe,expect,it} from "vitest";
import {buildFormData,validateBuildSource} from "./builds";

describe("source builds",()=>{
  it("accepts supported project archives and emits the Railpack contract",()=>{
    const file=new File(["source"],"experiment.tar.gz");
    expect(validateBuildSource("training image",file)).toBe("");
    const body=buildFormData(" training image ",file);
    expect(JSON.parse(String(body.get("metadata")))).toEqual({name:"training image",mode:"RAILPACK"});
    const stored=body.get("source") as File;
    expect(stored.name).toBe(file.name);
    expect(stored.size).toBe(file.size);
  });
  it("rejects missing, empty, and unsupported archives",()=>{
    expect(validateBuildSource("ok",null)).toContain("name");
    expect(validateBuildSource("valid name",new File([],"source.zip"))).toContain("empty");
    expect(validateBuildSource("valid name",new File(["x"],"source.txt"))).toContain(".zip");
  });
});
