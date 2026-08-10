import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

const tones: Record<string, string> = {
  RUNNING: "border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300",
  SUCCEEDED: "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  FAILED: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",
  LOST: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",
  CANCELLED: "border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",
  ONLINE: "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  DEGRADED: "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",
};
export function StatusBadge({ status }: { status: string }) {
  return <Badge variant="outline" className={cn("font-mono text-[10px]", tones[status])}>{status}</Badge>;
}
