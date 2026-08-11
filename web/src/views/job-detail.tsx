import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Download, Square, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { StatusBadge } from "@/components/status-badge";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { markJobSeen } from "@/lib/seen-jobs";
import { formatBytes, relative } from "@/lib/utils";
import type { User } from "@/types";
const terminal = new Set(["SUCCEEDED", "FAILED", "CANCELLED", "DELETED"]);

export function JobDetail({ user }: { user: User }) {
  const { id = "" } = useParams(), navigate = useNavigate(), qc = useQueryClient();
  const job = useQuery({ queryKey: ["job", id], queryFn: () => api.job(id), refetchInterval: 3000 });
  const logs = useQuery({ queryKey: ["logs", id], queryFn: async () => ({ stdout: await api.logs(id, "stdout"), stderr: await api.logs(id, "stderr") }), refetchInterval: 3000 });
  const events = useQuery({ queryKey: ["events", id], queryFn: () => api.events(id), refetchInterval: 3000 });
  useEffect(() => { if (job.data) markJobSeen(user.id, job.data); }, [job.data, user.id]);
  const stop = useMutation({ mutationFn: () => api.stopJob(id), onSuccess: () => { toast.success("Stop requested"); qc.invalidateQueries({ queryKey: ["job", id] }); }, onError: (error: Error) => toast.error(error.message) });
  const remove = useMutation({ mutationFn: () => api.deleteJob(id), onSuccess: () => { toast.success("Job deletion started"); navigate("/jobs"); }, onError: (error: Error) => toast.error(error.message) });
  if (!job.data) return <p className="text-sm text-muted-foreground">Loading job…</p>;
  const current = job.data;
  return <div className="space-y-5"><div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex items-center gap-3"><h1 className="text-xl font-semibold">{current.spec.name}</h1><StatusBadge status={current.status}/></div><p className="mt-1 font-mono text-xs text-muted-foreground">{current.id}</p></div><div className="flex gap-2"><Button asChild variant="outline" size="sm"><a href={`/api/v1/jobs/${id}/archive.zip`}><Download className="size-4"/>Download ZIP</a></Button>{!terminal.has(current.status) && <Button variant="outline" size="sm" onClick={() => stop.mutate()}><Square className="size-4"/>Stop</Button>}<AlertDialog><AlertDialogTrigger asChild><Button variant="destructive" size="sm" disabled={!terminal.has(current.status)}><Trash2 className="size-4"/>Delete</Button></AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete this job?</AlertDialogTitle><AlertDialogDescription>Logs and output files will be permanently removed. The audit tombstone remains.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction onClick={() => remove.mutate()}>Delete job</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></div></div>
    <Tabs defaultValue="overview"><TabsList><TabsTrigger value="overview">Overview</TabsTrigger><TabsTrigger value="logs">Logs</TabsTrigger><TabsTrigger value="metrics">Metrics</TabsTrigger><TabsTrigger value="events">Events</TabsTrigger></TabsList><TabsContent value="overview"><dl className="grid overflow-hidden rounded-md border sm:grid-cols-2 lg:grid-cols-4">{[["Image", current.spec.image], ["Node", current.assigned_node_id ?? "Unassigned"], ["Resources", `${current.spec.resources.cpu_millis / 1000} CPU · ${formatBytes(current.spec.resources.memory_bytes)} · ${current.spec.resources.gpu.count} GPU`], ["Created", relative(current.created_at)]].map(([label, value]) => <div key={label} className="border-b p-4 sm:border-r"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 break-all text-sm font-medium">{value}</dd></div>)}</dl>{current.failure_reason && <p className="mt-4 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{current.failure_reason}</p>}</TabsContent><TabsContent value="logs"><div className="grid gap-3 xl:grid-cols-2"><Log title="stdout" value={logs.data?.stdout}/><Log title="stderr" value={logs.data?.stderr}/></div></TabsContent><TabsContent value="metrics"><p className="rounded-md border p-8 text-center text-sm text-muted-foreground">Metrics will appear when the JobDock SDK reports them.</p></TabsContent><TabsContent value="events"><div className="divide-y rounded-md border">{(events.data ?? []).map((event) => <div key={event.id} className="grid grid-cols-[150px_1fr_auto] gap-3 p-3 text-sm"><span className="font-mono text-xs">{event.type}</span><span className="truncate text-muted-foreground">{JSON.stringify(event.payload)}</span><time className="text-xs text-muted-foreground">{relative(event.created_at)}</time></div>)}</div></TabsContent></Tabs><Button asChild variant="link" className="px-0"><Link to="/jobs">Back to jobs</Link></Button></div>;
}
function Log({ title, value }: { title: string; value?: string }) { return <div className="overflow-hidden rounded-md border"><div className="border-b bg-muted/40 px-3 py-2 font-mono text-xs">{title}</div><pre className="h-[55vh] overflow-auto bg-zinc-950 p-4 text-xs text-zinc-100">{value || "No output yet."}</pre></div>; }
