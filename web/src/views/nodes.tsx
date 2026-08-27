import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Copy, Pencil, Plus, Trash2 } from "lucide-react";
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
  const [installGPU, setInstallGPU] = useState(false);
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
  const installCommand = token ? agentInstallCommand(window.location.origin, token, installGPU) : "";
  return <div className="space-y-4">
    <div className="flex items-center justify-between"><h1 className="text-xl font-semibold">Nodes</h1>{user.role === "admin" && <Dialog><DialogTrigger asChild><Button size="sm" onClick={() => { setToken(""); setInstallGPU(false); api.enrollmentToken().then((value) => setToken(value.token)).catch((error) => toast.error(error.message)); }}><Plus className="size-4"/>Enroll node</Button></DialogTrigger><DialogContent className="sm:max-w-2xl"><DialogHeader><DialogTitle>Enroll a Docker host</DialogTitle><DialogDescription>Run one command on a Linux amd64 host within 15 minutes. Docker must already be available.</DialogDescription></DialogHeader><div className="flex gap-2"><Button size="sm" variant={installGPU ? "outline" : "default"} onClick={() => setInstallGPU(false)}>CPU</Button><Button size="sm" variant={installGPU ? "default" : "outline"} onClick={() => setInstallGPU(true)}>NVIDIA GPU</Button></div><div className="flex items-start gap-2"><code className="min-w-0 flex-1 break-all rounded bg-muted p-3 text-xs">{installCommand || "Generating…"}</code><Button variant="outline" size="icon" disabled={!installCommand} title="Copy install command" onClick={() => navigator.clipboard.writeText(installCommand).then(() => toast.success("Install command copied"))}><Copy className="size-4"/></Button></div><p className="text-xs text-muted-foreground">The command pulls the pinned agent image, persists its state, and consumes the one-time token. Treat it as a temporary credential.</p></DialogContent></Dialog>}</div>
    <div className="overflow-hidden rounded-md border"><Table><TableHeader><TableRow><TableHead>Node</TableHead><TableHead>Health</TableHead><TableHead>CPU reserved</TableHead><TableHead>Memory reserved</TableHead><TableHead>GPUs</TableHead><TableHead>Workspace free</TableHead>{user.role === "admin" && <TableHead/>}</TableRow></TableHeader><TableBody>{(query.data ?? []).map((node) => <TableRow key={node.id} className="cursor-pointer" tabIndex={0} onClick={() => navigate(`/nodes/${node.id}`)} onKeyDown={(event) => { if (event.key === "Enter") navigate(`/nodes/${node.id}`); }}><TableCell className="font-medium">{node.name}<div className="font-mono text-[10px] text-muted-foreground">{node.id.slice(0, 12)}</div></TableCell><TableCell><HeartbeatIndicator status={node.status} lastHeartbeat={node.last_heartbeat}/></TableCell><TableCell>{(node.cpu_allocated_millis / 1000).toFixed(1)} / {(node.cpu_total_millis / 1000).toFixed(1)} cores</TableCell><TableCell>{formatBytes(node.memory_allocated_bytes)} / {formatBytes(node.memory_total_bytes)}</TableCell><TableCell>{node.gpus.filter((gpu) => gpu.allocated).length} / {node.gpus.length}</TableCell><TableCell>{formatBytes(node.workspace_free_bytes)}</TableCell>{user.role === "admin" && <TableCell onClick={(event) => event.stopPropagation()}><div className="flex justify-end gap-1"><Button size="sm" variant="ghost" onClick={() => openEdit(node)}><Pencil className="size-4"/>Edit</Button><Button size="sm" variant="outline" onClick={() => action.mutate({ id: node.id, action: node.status === "DRAINING" ? "resume" : "drain" })}>{node.status === "DRAINING" ? "Resume" : "Drain"}</Button><AlertDialog><AlertDialogTrigger asChild><Button size="sm" variant="destructive" disabled={remove.isPending}><Trash2 className="size-4"/>Delete</Button></AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete {node.name}?</AlertDialogTitle><AlertDialogDescription>The node will be removed from scheduling and its agent credential will be revoked. Historical job attempts remain intact. Nodes with active jobs cannot be deleted.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction onClick={() => remove.mutate(node.id)}>Delete node</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></div></TableCell>}</TableRow>)}</TableBody></Table></div>
    <Dialog open={Boolean(editing)} onOpenChange={(open) => !open && setEditing(undefined)}><DialogContent><DialogHeader><DialogTitle>Edit node metadata</DialogTitle><DialogDescription>The effective name and labels override values reported by the agent.</DialogDescription></DialogHeader><div className="space-y-4"><div className="space-y-1.5"><Label htmlFor="node-name">Name</Label><Input id="node-name" maxLength={128} value={editName} onChange={(event) => setEditName(event.target.value)}/></div><div><Label className="mb-2 block">Labels</Label><KeyValueEditor rows={editLabels} onChange={setEditLabels} mode="labels"/></div></div><DialogFooter><Button variant="outline" onClick={() => setEditing(undefined)}>Cancel</Button><Button onClick={save} disabled={update.isPending}>{update.isPending ? "Saving…" : "Save changes"}</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
