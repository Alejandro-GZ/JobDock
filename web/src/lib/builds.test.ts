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
  it("persists explicit Dockerfile context without registry concepts",()=>{
    const file=new File(["source"],"project.zip");
    const body=buildFormData("custom image",file,"DOCKERFILE",{contextPath:"services/api",dockerfilePath:"docker/api.Dockerfile"});
    expect(JSON.parse(String(body.get("metadata")))).toEqual({name:"custom image",mode:"DOCKERFILE",context_path:"services/api",dockerfile_path:"docker/api.Dockerfile"});
  });
  it("rejects missing, empty, and unsupported archives",()=>{
    expect(validateBuildSource("ok",null)).toContain("name");
    expect(validateBuildSource("valid name",new File([],"source.zip"))).toContain("empty");
    expect(validateBuildSource("valid name",new File(["x"],"source.txt"))).toContain(".zip");
  });
});
