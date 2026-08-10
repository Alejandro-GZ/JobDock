import { Link } from "react-router-dom";
import { MoreHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { StatusBadge } from "@/components/status-badge";
import { formatBytes, relative } from "@/lib/utils";
import type { Job } from "@/types";

export function JobTable({ jobs }: { jobs: Job[] }) {
  return <div className="overflow-hidden rounded-md border"><Table><TableHeader><TableRow><TableHead>Job</TableHead><TableHead>Status</TableHead><TableHead>Node</TableHead><TableHead>Resources</TableHead><TableHead>Created</TableHead><TableHead className="w-10"/></TableRow></TableHeader><TableBody>
    {jobs.map((job) => <TableRow key={job.id}><TableCell><Link className="font-medium hover:underline" to={`/jobs/${job.id}`}>{job.spec.name}</Link><div className="font-mono text-[10px] text-muted-foreground">{job.id.slice(0, 12)}</div></TableCell><TableCell><StatusBadge status={job.status}/></TableCell><TableCell className="font-mono text-xs">{job.assigned_node_id?.slice(0, 10) ?? "—"}</TableCell><TableCell className="text-xs">{job.spec.resources.cpu_millis / 1000} CPU · {formatBytes(job.spec.resources.memory_bytes)}{job.spec.resources.gpu.count ? ` · ${job.spec.resources.gpu.count} GPU` : ""}</TableCell><TableCell className="text-muted-foreground">{relative(job.created_at)}</TableCell><TableCell><Button asChild variant="ghost" size="icon"><Link aria-label={`Open ${job.spec.name}`} to={`/jobs/${job.id}`}><MoreHorizontal className="size-4"/></Link></Button></TableCell></TableRow>)}
    {!jobs.length && <TableRow><TableCell colSpan={6} className="h-24 text-center text-muted-foreground">No jobs match this view.</TableCell></TableRow>}
  </TableBody></Table></div>;
}
