import { Check } from "lucide-react";
import type { ReactNode } from "react";
import { Progress } from "@/components/ui/progress";
import type { ProgressState } from "@/types";

export function ProgressWidget({ state, actions }: { state: ProgressState; actions?: ReactNode }) {
  const milestones = state.milestones ?? [], reached = state.reached ?? [];
  const primary = state.global_progress ?? state.simple?.value;
  if (primary == null && !state.current && milestones.length === 0) return null;
  const upcoming = milestones.filter(item => !reached.includes(item.name) && item.name !== state.current?.milestone);
  return <section className="h-full min-h-0 min-w-0 overflow-auto rounded-md border bg-card p-3" aria-label="Job progress">
    <div className="mb-2 flex items-center justify-between"><h3 className="text-sm font-medium">Progress</h3><div className="flex items-center gap-1">{primary != null && <span className="font-mono text-sm font-semibold">{Math.round(primary * 100)}%</span>}{actions}</div></div>
    {primary != null && <Progress value={primary * 100} className="h-2" aria-label="Global progress"/>}
    {state.current && <div className="mt-3"><div className="mb-1 flex justify-between text-xs"><span><span className="text-muted-foreground">Current stage · </span>{state.current.milestone || "Current stage"}</span><span>{Math.round(state.current.value * 100)}%</span></div><Progress value={state.current.value * 100} className="h-1.5" aria-label="Current stage progress"/></div>}
    {reached.length > 0 && <div className="mt-3 flex flex-wrap gap-2" aria-label="Reached milestones">{reached.map(name => <span key={name} className="flex items-center gap-1 rounded-full border px-2 py-1 text-[11px] text-muted-foreground"><Check className="size-3 text-emerald-600"/>{name}</span>)}</div>}
    {upcoming.length > 0 && <div className="mt-3"><p className="mb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Upcoming</p><ol className="flex flex-wrap gap-2">{upcoming.map(item => <li key={item.name} className="rounded-full border border-dashed px-2 py-1 text-[11px] text-muted-foreground">{item.name}{item.weight != null && <span className="ml-1 opacity-60">{item.weight}</span>}</li>)}</ol></div>}
  </section>;
}
