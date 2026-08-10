import { describe, expect, it } from "vitest";
import { pairsToRecord, validatePairs, type Pair } from "./key-value-editor";
const pair = (id: string, key: string, value = "value"): Pair => ({ id, key, value });
describe("key-value validation", () => {
  it("rejects duplicates and reserved environment variables", () => { const errors = validatePairs([pair("a", "PATH"), pair("b", "PATH"), pair("c", "JOBDOCK_TOKEN")], "environment"); expect(errors.b).toBe("Duplicate key"); expect(errors.c).toBe("JOBDOCK_* is reserved"); });
  it("creates a clean record", () => { expect(pairsToRecord([pair("a", " A ", "1"), pair("b", "", "")])).toEqual({ A: "1" }); });
});
