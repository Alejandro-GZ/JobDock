import {describe,expect,it} from "vitest";
import {bytesToUnit,memoryInputValue,unitToBytes} from "./memory-units";

describe("memory units",()=>{
  it("converts binary units without changing canonical bytes",()=>{const bytes=3*1024**3;expect(bytesToUnit(bytes,"GiB")).toBe(3);expect(bytesToUnit(bytes,"MiB")).toBe(3072);expect(unitToBytes(3072,"MiB")).toBe(bytes);expect(unitToBytes(bytes,"bytes")).toBe(bytes)});
  it("supports fractional values and rounds to an integer byte",()=>{expect(unitToBytes(1.5,"KiB")).toBe(1536);expect(memoryInputValue(1536,"KiB")).toBe("1.5")});
  it("rejects invalid numeric input safely",()=>{expect(unitToBytes(Number.NaN,"GiB")).toBe(0);expect(unitToBytes(-1,"bytes")).toBe(0)});
});
