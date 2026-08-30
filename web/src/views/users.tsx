import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Plus } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { relative } from "@/lib/utils";

export function Users() {
  const [open, setOpen] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("member");
  const queryClient = useQueryClient();
  const users = useQuery({ queryKey: ["users"], queryFn: api.users, refetchInterval: 5000 });
  const close = () => { setOpen(false); setUsername(""); setPassword(""); setRole("member"); };
  const create = useMutation({ mutationFn: () => api.createUser(username, password, role), onSuccess: () => { toast.success("User created"); close(); queryClient.invalidateQueries({ queryKey: ["users"] }); }, onError: (error: Error) => toast.error(error.message) });
  const createControl = <Dialog open={open} onOpenChange={value => value ? setOpen(true) : close()}><DialogTrigger asChild><Button size="sm"><Plus className="size-4"/>New user</Button></DialogTrigger><DialogContent><DialogHeader><DialogTitle>Create user</DialogTitle></DialogHeader><div className="space-y-4"><div><Label htmlFor="user-name">Username</Label><Input id="user-name" value={username} onChange={event => setUsername(event.target.value)}/></div><div><Label htmlFor="user-password">Password</Label><Input id="user-password" type="password" value={password} onChange={event => setPassword(event.target.value)}/></div><div><Label htmlFor="user-role">Role</Label><select id="user-role" className="mt-1 flex h-9 w-full rounded-md border bg-transparent px-3 text-sm" value={role} onChange={event => setRole(event.target.value)}><option value="member">Member</option><option value="admin">Administrator</option></select></div></div><DialogFooter><Button disabled={!username.trim() || !password || create.isPending} onClick={() => create.mutate()}>{create.isPending ? "Creating…" : "Create user"}</Button></DialogFooter></DialogContent></Dialog>;
  return <div className="overflow-hidden rounded-md border"><Table><TableHeader><TableRow><TableHead>User</TableHead><TableHead>Role</TableHead><TableHead>Last seen</TableHead><TableHead>Jobs running</TableHead><TableHead>Created</TableHead><TableHead className="text-right">{createControl}</TableHead></TableRow></TableHeader><TableBody>{(users.data ?? []).map(user => <TableRow key={user.id}><TableCell><div className="flex items-center gap-2.5"><span className="grid size-8 place-items-center rounded-full bg-muted text-xs font-semibold uppercase">{user.username.slice(0, 2)}</span><div><p className="font-medium">{user.username}</p><p className="font-mono text-[10px] text-muted-foreground">{user.id.slice(0, 12)}</p></div></div></TableCell><TableCell><Badge variant="outline" className="capitalize">{user.role}</Badge></TableCell><TableCell><span title={user.last_seen_at ? new Date(user.last_seen_at).toLocaleString() : "No recorded activity"}>{user.last_seen_at ? relative(user.last_seen_at) : "Never"}</span></TableCell><TableCell><div className="flex items-center gap-2"><Activity className={`size-3.5 ${(user.jobs_running ?? 0) > 0 ? "text-emerald-500" : "text-muted-foreground"}`}/><span className="tabular-nums">{user.jobs_running ?? 0}</span></div></TableCell><TableCell>{relative(user.created_at)}</TableCell><TableCell/></TableRow>)}</TableBody></Table></div>;
}
