import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export const statusVisuals: Record<string, { badge: string; dot: string }> = {
  QUEUED: {badge:"border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",dot:"bg-zinc-400"},
  ASSIGNED: {badge:"border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-300",dot:"bg-violet-500"},
  PULLING_IMAGE: {badge:"border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",dot:"bg-amber-500"},
  STARTING: {badge:"border-cyan-500/30 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300",dot:"bg-cyan-500"},
  STOPPING: {badge:"border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",dot:"bg-amber-500"},
  DELETING: {badge:"border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",dot:"bg-zinc-400"},
  RUNNING: {badge:"border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300",dot:"bg-blue-500"},
  SUCCEEDED: {badge:"border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",dot:"bg-emerald-500"},
  FAILED: {badge:"border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",dot:"bg-red-500"},
  LOST: {badge:"border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",dot:"bg-red-500"},
  CANCELLED: {badge:"border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",dot:"bg-zinc-400"},
  ONLINE: {badge:"border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",dot:"bg-emerald-500"},
  DEGRADED: {badge:"border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",dot:"bg-amber-500"},
};
export function StatusBadge({ status }: { status: string }) {
  const visual=statusVisuals[status];
  return <Badge variant="outline" className={cn("font-mono text-[10px]", visual?.badge)}>{status}</Badge>;
}
