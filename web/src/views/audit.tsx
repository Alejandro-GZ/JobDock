import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import {
  Activity, Boxes, Box, ChevronRight, CircleUserRound, Clipboard, Copy, Database,
  FileJson, Hammer, KeyRound, LogIn, RotateCcw, ScrollText, Search, Server, ShieldCheck, UserPlus, Users,
  type LucideIcon,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { auditActor, auditCategory, auditCategoryLabel, auditSummary, auditTarget, auditTargetHref, humanizeKey, type AuditCategory } from "@/lib/audit-events";
import { relative } from "@/lib/utils";
import type { AuditEvent, AuditQuery } from "@/types";

const categories: { value: AuditCategory; label: string }[] = [
  { value: "authentication", label: "Authentication" }, { value: "users", label: "Users" },
  { value: "jobs", label: "Jobs" }, { value: "builds", label: "Builds" }, { value: "nodes", label: "Nodes" },
  { value: "secrets", label: "Secrets" }, { value: "dashboards", label: "Dashboards" },
  { value: "tokens", label: "Tokens" }, { value: "system", label: "System" },
];
const targetTypes = ["user", "job", "build", "node", "secret", "dashboard", "personal_access_token", "enrollment_token"];
const categoryStyles: Record<AuditCategory, string> = {
  authentication: "bg-sky-500/10 text-sky-600 dark:text-sky-400", users: "bg-violet-500/10 text-violet-600 dark:text-violet-400",
  jobs: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400", builds: "bg-orange-500/10 text-orange-600 dark:text-orange-400",
  nodes: "bg-cyan-500/10 text-cyan-600 dark:text-cyan-400", secrets: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
  dashboards: "bg-fuchsia-500/10 text-fuchsia-600 dark:text-fuchsia-400", tokens: "bg-indigo-500/10 text-indigo-600 dark:text-indigo-400",
  system: "bg-slate-500/10 text-slate-600 dark:text-slate-400",
};
const categoryIcons: Record<AuditCategory, LucideIcon> = {
  authentication: LogIn, users: Users, jobs: Boxes, builds: Hammer, nodes: Server, secrets: KeyRound,
  dashboards: Activity, tokens: ShieldCheck, system: Database,
};

export function Audit() {
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState(params.get("q") ?? "");
  const [selected, setSelected] = useState<AuditEvent>();
  const filterKey = params.toString();
  const users = useQuery({ queryKey: ["users"], queryFn: api.users });

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const current = params.get("q") ?? "";
      if (search === current) return;
      const next = new URLSearchParams(params);
      search.trim() ? next.set("q", search.trim()) : next.delete("q");
      setParams(next, { replace: true });
    }, 250);
    return () => window.clearTimeout(timer);
  }, [search, params, setParams]);

  const queryFilters = useMemo<AuditQuery>(() => ({
    limit: 50,
    q: params.get("q") || undefined,
    category: params.get("category") || undefined,
    actor_id: params.get("actor") || undefined,
    target_type: params.get("target") || undefined,
    from: dateBoundary(params.get("from"), false),
    to: dateBoundary(params.get("to"), true),
  }), [filterKey]);
  const events = useInfiniteQuery({
    queryKey: ["audit", queryFilters],
    initialPageParam: undefined as number | undefined,
    queryFn: ({ pageParam }) => api.audit({ ...queryFilters, before: pageParam }),
    getNextPageParam: page => page.next_cursor,
  });
  const items = events.data?.pages.flatMap(page => page.items) ?? [];
  const grouped = groupEvents(items);
  const hasFilters = ["q", "category", "actor", "target", "from", "to"].some(key => params.has(key));

  const setFilter = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    value && value !== "all" ? next.set(key, value) : next.delete(key);
    setParams(next, { replace: true });
  };
  const clearFilters = () => { setSearch(""); setParams({}, { replace: true }); };

  return <div className="w-full space-y-4 pb-8">
    <div className="sticky top-0 z-10 rounded-lg border bg-background/95 p-2.5 shadow-sm backdrop-blur">
      <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-[minmax(14rem,0.9fr)_minmax(9rem,0.75fr)_minmax(9rem,0.75fr)_minmax(12.5rem,1fr)_9.5rem_9.5rem_auto]">
        <div className="relative"><Search className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-muted-foreground" /><Input aria-label="Search audit events" className="pl-8" value={search} onChange={event => setSearch(event.target.value)} placeholder="Search" /></div>
        <FilterSelect label="All categories" value={params.get("category") ?? "all"} onChange={value => setFilter("category", value)}>{categories.map(item => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</FilterSelect>
        <FilterSelect label="All actors" value={params.get("actor") ?? "all"} onChange={value => setFilter("actor", value)}><SelectItem value="system">System</SelectItem>{(users.data ?? []).map(user => <SelectItem key={user.id} value={user.id}>{user.username}</SelectItem>)}</FilterSelect>
        <FilterSelect label="All resources" value={params.get("target") ?? "all"} onChange={value => setFilter("target", value)}>{targetTypes.map(type => <SelectItem key={type} value={type}>{humanizeKey(type)}</SelectItem>)}</FilterSelect>
        <Input aria-label="Audit start date" title="From" type="date" value={params.get("from") ?? ""} onChange={event => setFilter("from", event.target.value)} />
        <Input aria-label="Audit end date" title="To" type="date" value={params.get("to") ?? ""} onChange={event => setFilter("to", event.target.value)} />
        <Button variant="ghost" size="sm" className="h-9" disabled={!hasFilters} onClick={clearFilters}><RotateCcw className="size-3.5" /><span className="xl:sr-only">Clear filters</span></Button>
      </div>
    </div>
    {events.isPending ? <AuditSkeleton /> : events.isError ? <EmptyState icon={ScrollText} title="Audit trail unavailable" detail={(events.error as Error).message} action={<Button variant="outline" size="sm" onClick={() => events.refetch()}>Try again</Button>} /> : items.length === 0 ? <EmptyState icon={Search} title={hasFilters ? "No matching activity" : "No audit activity yet"} detail={hasFilters ? "Adjust or clear the current filters." : "Administrative actions will appear here."} action={hasFilters ? <Button variant="outline" size="sm" onClick={clearFilters}>Clear filters</Button> : undefined} /> : <div className="space-y-5">
      {grouped.map(group => <section key={group.label} aria-labelledby={`audit-${group.key}`}><div className="mb-1.5 flex items-center gap-3"><h2 id={`audit-${group.key}`} className="shrink-0 text-xs font-semibold text-muted-foreground">{group.label}</h2><div className="h-px flex-1 bg-border" /></div><div className="overflow-hidden rounded-lg border bg-background">{group.events.map((event, index) => <AuditRow key={event.id} event={event} last={index === group.events.length - 1} onOpen={() => setSelected(event)} />)}</div></section>)}
      {events.hasNextPage && <div className="flex justify-center"><Button variant="outline" size="sm" disabled={events.isFetchingNextPage} onClick={() => events.fetchNextPage()}>{events.isFetchingNextPage ? "Loading…" : "Load older events"}</Button></div>}
    </div>}
    <AuditDetail event={selected} onOpenChange={open => { if (!open) setSelected(undefined); }} />
  </div>;
}

function FilterSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (value: string) => void; children: ReactNode }) {
  return <Select value={value} onValueChange={onChange}><SelectTrigger aria-label={label} className="h-9"><SelectValue placeholder={label} /></SelectTrigger><SelectContent><SelectItem value="all">{label}</SelectItem>{children}</SelectContent></Select>;
}

function AuditRow({ event, last, onOpen }: { event: AuditEvent; last: boolean; onOpen: () => void }) {
  const category = auditCategory(event), Icon = actionIcon(event) ?? categoryIcons[category], href = auditTargetHref(event);
  return <div role="button" tabIndex={0} aria-label={`Open audit event ${event.id}`} onClick={onOpen} onKeyDown={key => { if (key.key === "Enter" || key.key === " ") { key.preventDefault(); onOpen(); } }} className={`group grid cursor-pointer grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-3.5 py-3 outline-none transition-colors hover:bg-muted/45 focus-visible:bg-muted/45 ${last ? "" : "border-b"}`}>
    <div className={`grid size-8 place-items-center rounded-full ${categoryStyles[category]}`}><Icon className="size-3.5" /></div>
    <div className="min-w-0"><p className="truncate text-sm"><span className="font-medium">{auditSummary(event)}</span></p><div className="mt-0.5 flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground"><span>{auditCategoryLabel(event)}</span><span aria-hidden>·</span><code className="truncate">{event.action}</code>{href && <><span aria-hidden>·</span><Link className="truncate text-primary hover:underline" to={href} onClick={click => click.stopPropagation()}>{auditTarget(event)}</Link></>}</div></div>
    <div className="flex items-center gap-2 pl-2"><Tooltip><TooltipTrigger asChild><time className="whitespace-nowrap text-xs text-muted-foreground" dateTime={event.created_at}>{relative(event.created_at)}</time></TooltipTrigger><TooltipContent>{formatExact(event.created_at)}</TooltipContent></Tooltip><ChevronRight className="size-3.5 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" /></div>
  </div>;
}

function AuditDetail({ event, onOpenChange }: { event?: AuditEvent; onOpenChange: (open: boolean) => void }) {
  if (!event) return null;
  const category = auditCategory(event), Icon = categoryIcons[category], href = auditTargetHref(event), metadata = Object.entries(event.metadata ?? {});
  return <Sheet open onOpenChange={onOpenChange}><SheetContent><SheetHeader><div className="flex items-center gap-2"><div className={`grid size-8 place-items-center rounded-full ${categoryStyles[category]}`}><Icon className="size-4" /></div><div><SheetTitle>Audit event #{event.id}</SheetTitle><SheetDescription>{auditCategoryLabel(event)}</SheetDescription></div></div></SheetHeader><div className="min-h-0 flex-1 overflow-y-auto px-5 py-5">
    <p className="mb-5 text-base font-medium leading-relaxed">{auditSummary(event)}</p>
    <DetailSection title="Event"><Detail label="Action"><code>{event.action}</code></Detail><Detail label="Timestamp"><span>{formatExact(event.created_at)}</span><span className="block text-xs text-muted-foreground">{new Date(event.created_at).toISOString()}</span></Detail><Detail label="Event ID">{event.id}</Detail></DetailSection>
    <DetailSection title="Actor and target"><Detail label="Actor"><span>{auditActor(event)}</span>{event.actor_id && <IDValue value={event.actor_id} />}</Detail><Detail label="Target"><span>{auditTarget(event)}</span><IDValue value={event.target_id} />{href && <Button asChild variant="link" size="sm" className="h-auto p-0 text-xs"><Link to={href}>Open resource</Link></Button>}</Detail><Detail label="Resource type">{humanizeKey(event.target_type)}</Detail></DetailSection>
    <DetailSection title="Metadata">{metadata.length ? <dl className="divide-y rounded-md border">{metadata.map(([key, value]) => <div key={key} className="grid gap-1 px-3 py-2.5 sm:grid-cols-[9rem_minmax(0,1fr)]"><dt className="text-xs font-medium text-muted-foreground">{humanizeKey(key)}</dt><dd className="min-w-0 break-words text-xs"><MetadataValue value={value} /></dd></div>)}</dl> : <p className="text-sm text-muted-foreground">No additional metadata.</p>}</DetailSection>
    <details className="group rounded-md border"><summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-xs font-medium"><FileJson className="size-3.5" />Raw event JSON<ChevronRight className="ml-auto size-3.5 transition-transform group-open:rotate-90" /></summary><div className="border-t p-3"><pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded bg-muted p-3 text-[10px] leading-relaxed">{JSON.stringify(event, null, 2)}</pre><Button variant="outline" size="sm" className="mt-2" onClick={() => copy(JSON.stringify(event, null, 2), "Event JSON copied")}><Copy className="size-3.5" />Copy JSON</Button></div></details>
  </div></SheetContent></Sheet>;
}

function DetailSection({ title, children }: { title: string; children: ReactNode }) { return <section className="mb-6"><h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{title}</h3>{children}</section>; }
function Detail({ label, children }: { label: string; children: ReactNode }) { return <div className="mb-2 grid grid-cols-[7rem_minmax(0,1fr)] gap-3 text-sm"><span className="text-muted-foreground">{label}</span><div className="min-w-0 break-words">{children}</div></div>; }
function IDValue({ value }: { value: string }) { return <span className="mt-1 flex items-center gap-1"><code className="min-w-0 truncate text-[10px] text-muted-foreground">{value}</code><button className="shrink-0 text-muted-foreground hover:text-foreground" aria-label={`Copy ${value}`} onClick={() => copy(value, "ID copied")}><Copy className="size-3" /></button></span>; }

function MetadataValue({ value }: { value: unknown }): ReactNode {
  if (value === null || value === undefined) return <span className="text-muted-foreground">None</span>;
  if (typeof value === "boolean") return <Badge variant="outline">{value ? "Yes" : "No"}</Badge>;
  if (typeof value === "string") { const date = /^\d{4}-\d{2}-\d{2}T/.test(value) ? new Date(value) : undefined; return date && !Number.isNaN(date.getTime()) ? date.toLocaleString() : value; }
  if (typeof value === "number") return value.toLocaleString();
  if (Array.isArray(value)) return <span className="flex flex-wrap gap-1">{value.map((item, index) => <Badge key={index} variant="outline" className="font-normal">{typeof item === "object" ? JSON.stringify(item) : String(item)}</Badge>)}</span>;
  if (typeof value === "object") return <span className="grid gap-1">{Object.entries(value as Record<string, unknown>).map(([key, item]) => <span key={key}><span className="text-muted-foreground">{key}:</span> {typeof item === "object" ? JSON.stringify(item) : String(item)}</span>)}</span>;
  return String(value);
}

function EmptyState({ icon: Icon, title, detail, action }: { icon: LucideIcon; title: string; detail: string; action?: ReactNode }) { return <div className="grid min-h-64 place-items-center rounded-lg border border-dashed"><div className="flex max-w-sm flex-col items-center text-center"><Icon className="mb-3 size-7 text-muted-foreground" /><h2 className="text-sm font-medium">{title}</h2><p className="mt-1 text-xs text-muted-foreground">{detail}</p>{action && <div className="mt-4">{action}</div>}</div></div>; }
function AuditSkeleton() { return <div className="space-y-2 rounded-lg border p-3">{Array.from({ length: 7 }, (_, index) => <div key={index} className="flex items-center gap-3 py-2"><Skeleton className="size-8 rounded-full" /><div className="flex-1 space-y-2"><Skeleton className="h-3 w-2/3" /><Skeleton className="h-2.5 w-1/3" /></div><Skeleton className="h-3 w-14" /></div>)}</div>; }

function actionIcon(event: AuditEvent): LucideIcon | undefined { if (event.action === "user.create" || event.action === "user.bootstrap") return UserPlus; if (event.action === "auth.login") return CircleUserRound; if (event.action.includes("credential") || event.action.includes("pat.")) return KeyRound; if (event.action === "dashboard.report.export") return Clipboard; if (event.action.startsWith("job.")) return Box; return undefined; }
function groupEvents(events: AuditEvent[]) { const formatter = new Intl.DateTimeFormat("en", { weekday: "short", month: "short", day: "numeric", year: "numeric" }); const now = new Date(), previous = new Date(now); previous.setDate(previous.getDate() - 1); const today = localDateKey(now), yesterday = localDateKey(previous); const groups = new Map<string, AuditEvent[]>(); for (const event of events) { const key = localDateKey(new Date(event.created_at)); groups.set(key, [...(groups.get(key) ?? []), event]); } return Array.from(groups, ([key, groupedEvents]) => ({ key, label: key === today ? "Today" : key === yesterday ? "Yesterday" : formatter.format(new Date(`${key}T12:00:00`)), events: groupedEvents })); }
function localDateKey(date: Date) { return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`; }
function formatExact(value: string) { return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(value)); }
function dateBoundary(value: string | null, end: boolean) { if (!value) return undefined; return new Date(`${value}T${end ? "23:59:59.999" : "00:00:00"}`).toISOString(); }
async function copy(value: string, message: string) { try { await navigator.clipboard.writeText(value); toast.success(message); } catch { toast.error("Clipboard is unavailable"); } }
