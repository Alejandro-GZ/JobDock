import type { Job, JobAttempt } from "@/types";

export function attemptLabel(attempt: JobAttempt) {
  return `Attempt ${attempt.attempt_number} · ${attempt.status}`;
}

export function jobAtAttempt(job: Job, attempt: JobAttempt): Job {
  return {
    ...job,
    attempt_id: attempt.id,
    assigned_node_id: attempt.node_id,
    status: attempt.status,
    observed_status: attempt.status,
    image_digest: attempt.image_digest,
    exit_code: attempt.exit_code,
    failure_reason: attempt.failure_reason,
    started_at: attempt.started_at,
    finished_at: attempt.finished_at,
  };
}
