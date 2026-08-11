// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import { inputPath, jobFormData } from "./job-inputs";

describe("job inputs",()=>{it("builds relative multipart fields without embedding a client manifest",()=>{const file=new File(["data"],"value.txt");Object.defineProperty(file,"webkitRelativePath",{value:"dataset/value.txt"});const spec={name:"input job",image:"alpine",command:["true"],environment:{},secret_refs:[],resources:{cpu_millis:100,memory_bytes:1024,gpu:{count:0,min_vram_bytes:0}},labels:{},node_selector:{}};const body=jobFormData(spec,[file]);expect(inputPath(file)).toBe("dataset/value.txt");expect([...body.keys()]).toEqual(["spec","input:dataset/value.txt"]);expect(JSON.parse(String(body.get("spec")))).not.toHaveProperty("inputs")})});
