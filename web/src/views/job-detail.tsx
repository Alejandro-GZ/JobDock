import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Download, RotateCcw, Square, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { LiveLogs } from "@/components/live-logs";
import { MetricsPanel } from "@/components/metrics-panel";
import { StatusBadge } from "@/components/status-badge";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { markJobSeen } from "@/lib/seen-jobs";
import { attemptLabel, jobAtAttempt } from "@/lib/attempts";
import { formatBytes, relative } from "@/lib/utils";
import type { Job, JobAttempt, User } from "@/types";

const terminal = new Set(["SUCCEEDED", "FAILED", "CANCELLED", "DELETED"]);
const rerunnable = new Set(["SUCCEEDED", "FAILED", "CANCELLED"]);

export function JobDetail({ user }: { user: User }) {
  const { id = "" } = useParams(), navigate = useNavigate(), qc = useQueryClient();
  const [selectedAttemptID, setSelectedAttemptID] = useState("");
  const previousCurrentAttempt = useRef("");
  const job = useQuery({ queryKey: ["job", id], queryFn: () => api.job(id), refetchInterval: 3000 });
  const attempts = useQuery({ queryKey: ["job-attempts", id], queryFn: () => api.attempts(id), refetchInterval: 3000 });
  useEffect(() => {
    const currentAttempt = job.data?.attempt_id || "";
    setSelectedAttemptID(selected => currentAttempt && currentAttempt !== previousCurrentAttempt.current ? currentAttempt : selected || currentAttempt || attempts.data?.[0]?.id || "");
    previousCurrentAttempt.current = currentAttempt;
  }, [job.data?.attempt_id, attempts.data]);
  const selectedAttempt = attempts.data?.find(item => item.id === selectedAttemptID);
  const events = useQuery({ queryKey: ["events", id, selectedAttemptID], queryFn: () => api.events(id, 0, selectedAttemptID), enabled: !!selectedAttemptID, refetchInterval: 3000 });
  useEffect(() => { if (job.data) markJobSeen(user.id, job.data); }, [job.data, user.id]);

  const stop = useMutation({ mutationFn: () => api.stopJob(id), onSuccess: () => { toast.success("Stop requested"); qc.invalidateQueries({ queryKey: ["job", id] }); }, onError: (error: Error) => toast.error(error.message) });
  const rerun = useMutation({ mutationFn: () => api.rerunJob(id), onSuccess: () => { toast.success("Job queued for rerun"); qc.invalidateQueries({ queryKey: ["job", id] }); qc.invalidateQueries({ queryKey: ["job-attempts", id] }); }, onError: (error: Error) => toast.error(error.message) });
  const remove = useMutation({ mutationFn: () => api.deleteJob(id), onSuccess: () => { toast.success("Job deletion started"); navigate("/jobs"); }, onError: (error: Error) => toast.error(error.message) });

  const attemptJob = useMemo<Job | undefined>(() => {
    if (!job.data || !selectedAttempt) return job.data;
    return jobAtAttempt(job.data, selectedAttempt);
  }, [job.data, selectedAttempt]);

  if (!job.data) return <p className="text-sm text-muted-foreground">Loading job…</p>;
  const current = job.data;
  return <div className="space-y-5">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div><div className="flex items-center gap-3"><h1 className="text-xl font-semibold">{current.spec.name}</h1><StatusBadge status={current.status}/></div><p className="mt-1 font-mono text-xs text-muted-foreground">{current.id}</p></div>
      <div className="flex flex-wrap gap-2">
        {selectedAttempt && <Button asChild variant="outline" size="sm"><a href={`/api/v1/jobs/${id}/attempts/${selectedAttempt.id}/archive.zip`}><Download className="size-4"/>Attempt ZIP</a></Button>}
        {rerunnable.has(current.status) && <Button variant="outline" size="sm" onClick={() => rerun.mutate()} disabled={rerun.isPending}><RotateCcw className="size-4"/>Rerun</Button>}
        {!terminal.has(current.status) && current.status !== "QUEUED" && <Button variant="outline" size="sm" onClick={() => stop.mutate()}><Square className="size-4"/>Stop</Button>}
        <AlertDialog><AlertDialogTrigger asChild><Button variant="destructive" size="sm" disabled={!terminal.has(current.status)}><Trash2 className="size-4"/>Delete</Button></AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete this job?</AlertDialogTitle><AlertDialogDescription>All attempts, logs and output files will be permanently removed. The audit tombstone remains.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction onClick={() => remove.mutate()}>Delete job</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
      </div>
    </div>

    <div className="flex flex-wrap items-center gap-3 rounded-md border bg-muted/20 px-3 py-2">
      <label htmlFor="attempt" className="text-xs font-medium">Execution</label>
      <select id="attempt" className="h-8 min-w-56 rounded-md border bg-background px-2 text-sm" value={selectedAttemptID} onChange={event => setSelectedAttemptID(event.target.value)} disabled={!attempts.data?.length}>
        {!attempts.data?.length && <option value="">No attempts yet</option>}
        {(attempts.data ?? []).map(item => <option key={item.id} value={item.id}>{attemptLabel(item)}</option>)}
      </select>
      {selectedAttempt && <><StatusBadge status={selectedAttempt.status}/><span className="font-mono text-xs text-muted-foreground">{selectedAttempt.id}</span></>}
      {current.status === "QUEUED" && !current.attempt_id && <span className="text-xs text-muted-foreground">The next numbered attempt is waiting for capacity.</span>}
    </div>

    <Tabs defaultValue="overview"><TabsList><TabsTrigger value="overview">Overview</TabsTrigger><TabsTrigger value="logs" disabled={!selectedAttempt}>Logs</TabsTrigger><TabsTrigger value="metrics" disabled={!selectedAttempt}>Metrics</TabsTrigger><TabsTrigger value="events" disabled={!selectedAttempt}>Events</TabsTrigger></TabsList>
      <TabsContent value="overview"><AttemptOverview job={current} selected={selectedAttempt} attempts={attempts.data ?? []}/></TabsContent>
      <TabsContent value="logs">{selectedAttempt && <LiveLogs jobId={id} attemptId={selectedAttempt.id}/>}</TabsContent>
      <TabsContent value="metrics">{attemptJob && selectedAttempt && <MetricsPanel job={attemptJob}/>}</TabsContent>
      <TabsContent value="events"><div className="divide-y rounded-md border">{(events.data ?? []).map(event => <div key={event.id} className="grid grid-cols-[150px_1fr_auto] gap-3 p-3 text-sm"><span className="font-mono text-xs">{event.type}</span><span className="truncate text-muted-foreground">{JSON.stringify(event.payload)}</span><time className="text-xs text-muted-foreground">{relative(event.created_at)}</time></div>)}</div></TabsContent>
    </Tabs>
    <Button asChild variant="link" className="px-0"><Link to="/jobs">Back to jobs</Link></Button>
  </div>;
}

function AttemptOverview({ job, selected, attempts }: { job: Job; selected?: JobAttempt; attempts: JobAttempt[] }) {
  const outputBytes = selected?.outputs.reduce((total, output) => total + output.size, 0) ?? 0;
  return <div className="space-y-4">
    <dl className="grid overflow-hidden rounded-md border sm:grid-cols-2 lg:grid-cols-6">{[
      ["Image", selected?.image_digest || job.spec.image],
      ["Node", selected?.node_id ?? "Unassigned"],
      ["Resources", `${job.spec.resources.cpu_millis / 1000} CPU · ${formatBytes(job.spec.resources.memory_bytes)} · ${job.spec.resources.gpu.count} GPU`],
      ["Exit code", selected?.exit_code ?? "—"],
      ["Outputs", `${selected?.outputs.length ?? 0} files · ${formatBytes(outputBytes)}`],
      ["Finished", selected?.finished_at ? relative(selected.finished_at) : "—"],
    ].map(([label, value]) => <div key={label} className="border-b p-4 sm:border-r"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 break-all text-sm font-medium">{value}</dd></div>)}</dl>
    {selected?.failure_reason && <p className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{selected.failure_reason}</p>}
    {!!selected?.outputs.length && <section><h2 className="mb-2 text-sm font-semibold">Outputs from attempt {selected.attempt_number}</h2><div className="divide-y rounded-md border">{selected.outputs.map(output => <div key={output.path} className="grid grid-cols-[1fr_auto] gap-3 p-3 text-xs"><span className="font-mono">{output.path}</span><span className="text-muted-foreground">{formatBytes(output.size)} · sha256:{output.sha256.slice(0, 12)}</span></div>)}</div></section>}
    <section><h2 className="mb-2 text-sm font-semibold">Attempt history</h2><div className="overflow-x-auto rounded-md border"><table className="w-full text-left text-xs"><thead className="border-b bg-muted/40 text-muted-foreground"><tr><th className="p-3">Attempt</th><th className="p-3">Status</th><th className="p-3">Node</th><th className="p-3">Started</th><th className="p-3">Duration</th><th className="p-3">Exit</th><th className="p-3">Outputs</th></tr></thead><tbody>{attempts.map(attempt => <tr key={attempt.id} className="border-b last:border-0"><td className="p-3 font-medium">#{attempt.attempt_number}</td><td className="p-3"><StatusBadge status={attempt.status}/></td><td className="p-3 font-mono">{attempt.node_id}</td><td className="p-3">{attempt.started_at ? relative(attempt.started_at) : "—"}</td><td className="p-3">{duration(attempt)}</td><td className="p-3">{attempt.exit_code ?? "—"}</td><td className="p-3">{attempt.outputs.length}</td></tr>)}</tbody></table></div></section>
    {job.spec.inputs?.length ? <section><h2 className="mb-2 text-sm font-semibold">Reused immutable inputs</h2><div className="divide-y rounded-md border">{job.spec.inputs.map(input => <div key={input.path} className="grid grid-cols-[1fr_auto] gap-3 p-3 text-xs"><span className="font-mono">{input.path}</span><span className="text-muted-foreground">{formatBytes(input.size)} · sha256:{input.sha256.slice(0, 12)}</span></div>)}</div></section> : null}
  </div>;
}

function duration(attempt: JobAttempt) {
  if (!attempt.started_at) return "—";
  const end = attempt.finished_at ? Date.parse(attempt.finished_at) : Date.now();
  const seconds = Math.max(0, Math.round((end - Date.parse(attempt.started_at)) / 1000));
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}
