import { describe, expect, it } from "vitest";
import { attemptLabel, jobAtAttempt } from "@/lib/attempts";
import type { Job, JobAttempt } from "@/types";

describe("attempt presentation", () => {
  it("uses historical execution identity without changing the reusable spec", () => {
    const job = { id: "job", owner_id: "owner", status: "QUEUED", desired_status: "RUNNING", observed_status: "QUEUED", created_at: "2026-08-13T10:00:00Z", version: 4, spec: { name: "repeatable", image: "alpine", command: ["true"], environment: {}, secret_refs: [], resources: { cpu_millis: 100, memory_bytes: 1024, gpu: { count: 0, min_vram_bytes: 0 } }, labels: {}, node_selector: {} } } satisfies Job;
    const attempt = { id: "attempt-two", job_id: job.id, attempt_number: 2, node_id: "node-b", status: "FAILED", exit_code: 7, outputs: [], created_at: "2026-08-13T11:00:00Z", started_at: "2026-08-13T11:00:01Z", finished_at: "2026-08-13T11:00:02Z" } satisfies JobAttempt;
    const view = jobAtAttempt(job, attempt);
    expect(attemptLabel(attempt)).toBe("Attempt 2 · FAILED");
    expect(view).toMatchObject({ attempt_id: attempt.id, assigned_node_id: "node-b", status: "FAILED", exit_code: 7 });
    expect(view.spec).toBe(job.spec);
  });
});
