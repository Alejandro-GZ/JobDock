import { describe, expect, it } from "vitest";
import { appendVisibleLog, decodeBase64Bytes } from "./live-logs";

describe("live log buffering", () => {
  it("appends only new text and bounds browser memory", () => {
    const first = appendVisibleLog({ text: "abc", truncated: false }, "def", 5);
    expect(first).toEqual({ text: "bcdef", truncated: true });
    expect(appendVisibleLog(first, "g", 5)).toEqual({ text: "cdefg", truncated: true });
  });

  it("decodes the byte-safe SSE payload", () => {
    expect(new TextDecoder().decode(decodeBase64Bytes("aGVsbG8="))).toBe("hello");
  });
});
