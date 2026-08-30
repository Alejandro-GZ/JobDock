import { describe, expect, it } from "vitest";
import { auditCategory, auditSummary, auditTargetHref, humanizeAction } from "./audit-events";
import type { AuditEvent } from "@/types";

const event = (update: Partial<AuditEvent> = {}): AuditEvent => ({
  id: 12,
  actor_id: "actor-id",
  actor_label: "Alex",
  action: "job.create",
  target_type: "job",
  target_id: "job-id",
  target_label: "Training run",
  target_available: true,
  metadata: {},
  created_at: "2026-08-30T12:00:00Z",
  ...update,
});

describe("audit event presentation", () => {
  it("translates known events into readable summaries and links", () => {
    expect(auditSummary(event())).toBe("Alex created job Training run");
    expect(auditCategory(event())).toBe("jobs");
    expect(auditTargetHref(event())).toBe("/jobs/job-id");
  });

  it("uses immutable labels and suppresses links to deleted resources", () => {
    const deleted = event({ action: "build.delete", target_type: "build", target_label: "Release image", target_available: false });
    expect(auditSummary(deleted)).toBe("Alex deleted build Release image");
    expect(auditTargetHref(deleted)).toBeUndefined();
  });

  it("handles system and unknown actions without exposing markup", () => {
    expect(auditSummary(event({ actor_id: undefined, actor_label: "System", action: "future.operation", target_label: "<script>alert(1)</script>" }))).toBe("System future operation <script>alert(1)</script>");
    expect(humanizeAction("future.operation_name")).toBe("future operation name");
  });
});
