import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Copy, Cpu, Pencil, Plus, RefreshCw, ShieldAlert, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { HeartbeatIndicator } from "@/components/heartbeat-indicator";
import { KeyValueEditor, pairsToRecord, validatePairs, type Pair } from "@/components/key-value-editor";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatBytes } from "@/lib/utils";
import { agentInstallCommand } from "@/lib/agent-install";
import type { Node, User } from "@/types";

const labelRows = (node: Node): Pair[] => Object.entries(node.labels).map(([key, value], index) => ({ id: `${node.id}-${index}`, key, value }));

export function Nodes({ user }: { user: User }) {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [token, setToken] = useState("");
  const [tokenExpiresAt, setTokenExpiresAt] = useState("");
  const [enrollOpen, setEnrollOpen] = useState(false);
  const [editing, setEditing] = useState<Node>();
  const [editName, setEditName] = useState("");
  const [editLabels, setEditLabels] = useState<Pair[]>([]);
  const query = useQuery({ queryKey: ["nodes"], queryFn: api.nodes, refetchInterval: 2000 });
  const qc = useQueryClient();
  useEffect(() => { const requested = params.get("node"); if (requested) navigate(`/nodes/${requested}`, { replace: true }); }, [navigate, params]);
  const action = useMutation({ mutationFn: ({ id, action }: { id: string; action: "drain" | "resume" }) => api.setNode(id, action), onSuccess: (_, value) => { toast.success(`Node ${value.action === "drain" ? "drained" : "resumed"}`); qc.invalidateQueries({ queryKey: ["nodes"] }); }, onError: (error: Error) => toast.error(error.message) });
  const update = useMutation({ mutationFn: () => api.updateNode(editing!.id, editName.trim(), pairsToRecord(editLabels)), onSuccess: () => { toast.success("Node metadata updated"); setEditing(undefined); qc.invalidateQueries({ queryKey: ["nodes"] }); }, onError: (error: Error) => toast.error(error.message) });
  const remove = useMutation({ mutationFn: (id: string) => api.deleteNode(id), onSuccess: () => { toast.success("Node deleted"); setEditing(undefined); qc.invalidateQueries({ queryKey: ["nodes"] }); }, onError: (error: Error) => toast.error(error.message) });
  const openEdit = (node: Node) => { setEditing(node); setEditName(node.name); setEditLabels(labelRows(node)); };
  const save = () => { if (!editName.trim()) return toast.error("Node name is required"); if (Object.keys(validatePairs(editLabels, "labels")).length) return toast.error("Fix the label errors"); update.mutate(); };
  const issueToken = async () => { setToken(""); try { const value = await api.enrollmentToken(); setToken(value.token); setTokenExpiresAt(value.expires_at); } catch (error) { toast.error((error as Error).message); } };
  const revokeToken = async (notify = true) => { const current = token; setToken(""); setTokenExpiresAt(""); if (!current) return; try { await api.revokeEnrollmentToken(current); if (notify) toast.success("Enrollment token revoked"); } catch (error) { if (notify) toast.error((error as Error).message); } };
  const regenerateToken = async () => { await revokeToken(false); await issueToken(); toast.success("Enrollment token regenerated"); };
  const setEnrollmentOpen = (open: boolean) => { setEnrollOpen(open); if (open) void issueToken(); else void revokeToken(false); };
  const copyInstall = (command: string) => navigator.clipboard.writeText(command).then(() => toast.success("Install command copied"), () => toast.error("Clipboard is unavailable"));
  const cpuCommand = token ? agentInstallCommand(window.location.origin, token, false) : "";
  const gpuCommand = token ? agentInstallCommand(window.location.origin, token, true) : "";
  const commandCard = (kind: "CPU" | "NVIDIA GPU", command: string) => <div className="space-y-2 rounded-md border p-3"><div className="flex items-center gap-2 text-sm font-medium">{kind === "CPU" ? <Cpu className="size-4"/> : <span className="size-4 rounded-sm bg-emerald-500"/>}{kind}</div><div className="flex items-start gap-2"><code className="min-w-0 flex-1 break-all rounded bg-muted p-3 text-xs">{command || "Generating a one-time command…"}</code><Button variant="outline" size="icon" disabled={!command} title={`Copy ${kind} install command`} onClick={() => void copyInstall(command)}><Copy className="size-4"/></Button></div></div>;
  const enrollControl = <Dialog open={enrollOpen} onOpenChange={setEnrollmentOpen}><DialogTrigger asChild><Button size="sm"><Plus className="size-4"/>Enroll node</Button></DialogTrigger><DialogContent className="sm:max-w-3xl"><DialogHeader><DialogTitle>Enroll a Docker host</DialogTitle><DialogDescription>Choose the command for a Linux amd64 CPU or NVIDIA host. It installs the agent matching this JobDock release and waits for a confirmed heartbeat.</DialogDescription></DialogHeader><div className="space-y-3">{commandCard("CPU", cpuCommand)}{commandCard("NVIDIA GPU", gpuCommand)}</div><div className="flex items-start gap-2 rounded-md bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-300"><ShieldAlert className="mt-0.5 size-4 shrink-0"/><span>These commands contain a temporary credential. Avoid shared terminals, shell recording, chat, and command history. The token expires {tokenExpiresAt ? new Date(tokenExpiresAt).toLocaleString() : "in 15 minutes"} and can be used only once.</span></div><DialogFooter className="sm:justify-between"><Button variant="ghost" onClick={() => void revokeToken()} disabled={!token}><X className="size-4"/>Revoke</Button><div className="flex gap-2"><Button variant="outline" onClick={() => void regenerateToken()}><RefreshCw className="size-4"/>Regenerate</Button><Button onClick={() => setEnrollmentOpen(false)}>Done</Button></div></DialogFooter></DialogContent></Dialog>;
  return <div className="space-y-4">
    <div className="overflow-hidden rounded-md border"><Table><TableHeader><TableRow><TableHead>Node</TableHead><TableHead>Health</TableHead><TableHead>CPU reserved</TableHead><TableHead>Memory reserved</TableHead><TableHead>GPUs</TableHead><TableHead>Workspace free</TableHead>{user.role === "admin" && <TableHead className="text-right">{enrollControl}</TableHead>}</TableRow></TableHeader><TableBody>{(query.data ?? []).map((node) => <TableRow key={node.id} className="cursor-pointer" tabIndex={0} onClick={() => navigate(`/nodes/${node.id}`)} onKeyDown={(event) => { if (event.key === "Enter") navigate(`/nodes/${node.id}`); }}><TableCell className="font-medium">{node.name}<div className="font-mono text-[10px] text-muted-foreground">{node.id.slice(0, 12)}</div></TableCell><TableCell><HeartbeatIndicator status={node.status} lastHeartbeat={node.last_heartbeat}/></TableCell><TableCell>{(node.cpu_allocated_millis / 1000).toFixed(1)} / {(node.cpu_total_millis / 1000).toFixed(1)} cores</TableCell><TableCell>{formatBytes(node.memory_allocated_bytes)} / {formatBytes(node.memory_total_bytes)}</TableCell><TableCell>{node.gpus.filter((gpu) => gpu.allocated).length} / {node.gpus.length}</TableCell><TableCell>{formatBytes(node.workspace_free_bytes)}</TableCell>{user.role === "admin" && <TableCell onClick={(event) => event.stopPropagation()}><div className="flex justify-end gap-1"><Button size="sm" variant="ghost" onClick={() => openEdit(node)}><Pencil className="size-4"/>Edit</Button><Button size="sm" variant="outline" onClick={() => action.mutate({ id: node.id, action: node.status === "DRAINING" ? "resume" : "drain" })}>{node.status === "DRAINING" ? "Resume" : "Drain"}</Button><AlertDialog><AlertDialogTrigger asChild><Button size="sm" variant="destructive" disabled={remove.isPending}><Trash2 className="size-4"/>Delete</Button></AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete {node.name}?</AlertDialogTitle><AlertDialogDescription>The node will be removed from scheduling and its agent credential will be revoked. Historical job attempts remain intact. Nodes with active jobs cannot be deleted.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction onClick={() => remove.mutate(node.id)}>Delete node</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></div></TableCell>}</TableRow>)}</TableBody></Table></div>
    <Dialog open={Boolean(editing)} onOpenChange={(open) => !open && setEditing(undefined)}><DialogContent><DialogHeader><DialogTitle>Edit node metadata</DialogTitle><DialogDescription>The effective name and labels override values reported by the agent.</DialogDescription></DialogHeader><div className="space-y-4"><div className="space-y-1.5"><Label htmlFor="node-name">Name</Label><Input id="node-name" maxLength={128} value={editName} onChange={(event) => setEditName(event.target.value)}/></div><div><Label className="mb-2 block">Labels</Label><KeyValueEditor rows={editLabels} onChange={setEditLabels} mode="labels"/></div></div><DialogFooter><Button variant="outline" onClick={() => setEditing(undefined)}>Cancel</Button><Button onClick={save} disabled={update.isPending}>{update.isPending ? "Saving…" : "Save changes"}</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
