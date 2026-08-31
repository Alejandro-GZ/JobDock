import type { AuditEvent } from "@/types";

export type AuditCategory = "authentication" | "users" | "jobs" | "builds" | "nodes" | "secrets" | "dashboards" | "tokens" | "system";

type Descriptor = { category: AuditCategory; verb: (event: AuditEvent) => string };

const descriptors: Record<string, Descriptor> = {
  "auth.login": { category: "authentication", verb: () => "signed in" },
  "auth.pat.create": { category: "tokens", verb: () => "created personal access token" },
  "auth.pat.revoke": { category: "tokens", verb: () => "revoked personal access token" },
  "user.bootstrap": { category: "users", verb: () => "bootstrapped administrator" },
  "user.setup": { category: "users", verb: () => "completed first-run setup for administrator" },
  "user.create": { category: "users", verb: () => "created user" },
  "job.create": { category: "jobs", verb: () => "created job" },
  "job.stop": { category: "jobs", verb: () => "requested job to stop" },
  "job.rerun": { category: "jobs", verb: () => "reran job" },
  "job.delete": { category: "jobs", verb: () => "deleted job" },
  "build.create": { category: "builds", verb: () => "created build" },
  "build.confirm": { category: "builds", verb: () => "started build" },
  "build.cancel": { category: "builds", verb: () => "cancelled build" },
  "build.delete": { category: "builds", verb: () => "deleted build" },
  "node.enroll": { category: "nodes", verb: () => "enrolled node" },
  "node.credential.rotate": { category: "nodes", verb: () => "rotated credentials for node" },
  "node.metadata.update": { category: "nodes", verb: () => "updated node" },
  "node.drain": { category: "nodes", verb: () => "drained node" },
  "node.resume": { category: "nodes", verb: () => "resumed node" },
  "node.delete": { category: "nodes", verb: () => "removed node" },
  "node.enrollment_token.create": { category: "nodes", verb: () => "created a node enrollment token" },
  "node.enrollment_token.revoke": { category: "nodes", verb: () => "revoked a node enrollment token" },
  "secret.create": { category: "secrets", verb: () => "created secret" },
  "secret.delete": { category: "secrets", verb: () => "deleted secret" },
  "dashboard.create": { category: "dashboards", verb: () => "created dashboard" },
  "dashboard.duplicate": { category: "dashboards", verb: () => "duplicated dashboard" },
  "dashboard.update": { category: "dashboards", verb: () => "updated dashboard" },
  "dashboard.delete": { category: "dashboards", verb: () => "deleted dashboard" },
  "dashboard.template.apply": { category: "dashboards", verb: event => `applied template ${metadataText(event, "template_id", "")} to` },
  "dashboard.report.export": { category: "dashboards", verb: () => "exported dashboard report for job" },
};

const categoryLabels: Record<AuditCategory, string> = {
  authentication: "Authentication", users: "Users", jobs: "Jobs", builds: "Builds", nodes: "Nodes",
  secrets: "Secrets", dashboards: "Dashboards", tokens: "Tokens", system: "System",
};

export function auditCategory(event: AuditEvent): AuditCategory {
  if (descriptors[event.action]) return descriptors[event.action].category;
  if (!event.actor_id) return "system";
  const prefix = event.action.split(".")[0];
  return ({ auth: "authentication", user: "users", job: "jobs", build: "builds", node: "nodes", secret: "secrets", dashboard: "dashboards" } as Record<string, AuditCategory>)[prefix] ?? "system";
}

export function auditCategoryLabel(event: AuditEvent) { return categoryLabels[auditCategory(event)]; }

export function auditActor(event: AuditEvent) { return event.actor_label || (event.actor_id ? shortID(event.actor_id) : "System"); }
export function auditTarget(event: AuditEvent) { return event.target_label || metadataText(event, "name", "") || metadataText(event, "username", "") || shortID(event.target_id); }

export function auditSummary(event: AuditEvent) {
  const descriptor = descriptors[event.action];
  const verb = descriptor?.verb(event) ?? humanizeAction(event.action);
  const target = auditTarget(event);
  if (event.action === "auth.login") return `${auditActor(event)} signed in`;
  if (event.action === "node.enrollment_token.create" || event.action === "node.enrollment_token.revoke") return `${auditActor(event)} ${verb}`;
  return `${auditActor(event)} ${verb} ${target}`.replace(/\s+/g, " ").trim();
}

export function auditTargetHref(event: AuditEvent) {
  if (!event.target_available) return undefined;
  if (event.target_type === "job") return `/jobs/${event.target_id}`;
  if (event.target_type === "build") return `/builds/${event.target_id}`;
  if (event.target_type === "node") return `/nodes/${event.target_id}`;
  if (event.target_type === "dashboard") {
    const jobID = metadataText(event, "job_id", "");
    return jobID ? `/jobs/${jobID}` : undefined;
  }
  if (event.target_type === "personal_access_token") return "/tokens";
  if (event.target_type === "secret") return "/secrets";
  if (event.target_type === "user") return "/users";
  return undefined;
}

export function humanizeKey(value: string) {
  return value.replace(/_/g, " ").replace(/\b\w/g, letter => letter.toUpperCase());
}

export function humanizeAction(value: string) {
  return value.split(".").map(part => part.replace(/_/g, " ")).join(" ");
}

export function shortID(value: string) { return value.length > 12 ? `${value.slice(0, 12)}…` : value; }

function metadataText(event: AuditEvent, key: string, fallback: string) {
  const value = event.metadata?.[key];
  return typeof value === "string" && value.trim() ? value : fallback;
}
