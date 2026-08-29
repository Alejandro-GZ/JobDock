import type { TablePage } from "@/types";

export type OfflineLogFragment = {
  stream: "stdout" | "stderr";
  text: string;
};

type OfflineDashboardSnapshot = {
  attemptID: string;
  tables: Record<string, TablePage>;
  logFragments: OfflineLogFragment[];
};

let snapshot: OfflineDashboardSnapshot | undefined;

export function configureOfflineDashboardSnapshot(value: OfflineDashboardSnapshot) {
  snapshot = value;
}

export function clearOfflineDashboardSnapshot() {
  snapshot = undefined;
}

export function offlineDashboardTable(attemptID: string, name: string, query: string) {
  const source = snapshot?.attemptID === attemptID ? snapshot.tables[name] : undefined;
  if (!source) return undefined;
  const params = new URLSearchParams(query), filters = params.getAll("filter"), sort = params.get("sort"), descending = params.get("order") === "desc", absolute = params.get("absolute") === "true";
  const offset = nonNegativeInteger(params.get("offset"), 0), limit = Math.max(1, nonNegativeInteger(params.get("limit"), source.items.length || 1));
  let items = source.items.filter(item => filters.every(filter => {
    const separator = filter.indexOf("=");
    if (separator < 1) return true;
    const key = filter.slice(0, separator), expected = filter.slice(separator + 1).toLocaleLowerCase();
    return String(item.values[key] ?? "").toLocaleLowerCase().includes(expected);
  }));
  if (sort) items = [...items].sort((left, right) => compare(left.values[sort], right.values[sort], absolute) * (descending ? -1 : 1));
  const total = items.length, page = items.slice(offset, offset + limit);
  return { ...source, items: page, total, next_cursor: offset + page.length < total ? page.at(-1)?.cursor : undefined } satisfies TablePage;
}

export function offlineDashboardLogFragments(attemptID: string, streams: readonly ("stdout" | "stderr")[]) {
  if (snapshot?.attemptID !== attemptID) return undefined;
  const selected = new Set(streams);
  return snapshot.logFragments.filter(fragment => selected.has(fragment.stream));
}

function nonNegativeInteger(value: string | null, fallback: number) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : fallback;
}

function compare(left: unknown, right: unknown, absolute: boolean) {
  if (typeof left === "number" && typeof right === "number") {
    const a = absolute ? Math.abs(left) : left, b = absolute ? Math.abs(right) : right;
    return a === b ? 0 : a < b ? -1 : 1;
  }
  return String(left ?? "").localeCompare(String(right ?? ""), undefined, { numeric: true, sensitivity: "base" });
}
