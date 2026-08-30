import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { relative } from "@/lib/utils";

type SecretKind = "generic" | "registry";

export function Secrets() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [kind, setKind] = useState<SecretKind>("generic");
  const [registryServer, setRegistryServer] = useState("");
  const [registryUsername, setRegistryUsername] = useState("");
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ["secrets"], queryFn: api.secrets });
  const reset = () => { setName(""); setValue(""); setKind("generic"); setRegistryServer(""); setRegistryUsername(""); };
  const close = () => { setOpen(false); reset(); };
  const create = useMutation({
    mutationFn: () => api.createSecret(name.trim(), kind === "registry" ? JSON.stringify({ serveraddress: registryServer.trim(), username: registryUsername.trim(), password: value }) : value, kind),
    onSuccess: () => { toast.success("Secret created"); close(); queryClient.invalidateQueries({ queryKey: ["secrets"] }); },
    onError: (error: Error) => toast.error(error.message),
  });
  const remove = useMutation({ mutationFn: api.deleteSecret, onSuccess: () => { toast.success("Secret deleted"); queryClient.invalidateQueries({ queryKey: ["secrets"] }); }, onError: (error: Error) => toast.error(error.message) });
  const valid = name.trim() && value && (kind === "generic" || (registryServer.trim() && registryUsername.trim()));

  return <div className="space-y-4">
    <div className="flex justify-between"><h1 className="text-xl font-semibold">Secrets</h1><Dialog open={open} onOpenChange={next => next ? setOpen(true) : close()}><DialogTrigger asChild><Button size="sm"><Plus className="size-4" />New secret</Button></DialogTrigger><DialogContent><DialogHeader><DialogTitle>Create secret</DialogTitle><DialogDescription>Secret values are encrypted and cannot be retrieved after creation.</DialogDescription></DialogHeader><div className="space-y-4">
      <div><Label htmlFor="secret-name">Name</Label><Input id="secret-name" value={name} onChange={event => setName(event.target.value)} placeholder="model-api-token" /></div>
      <div><Label htmlFor="secret-type">Type</Label><select id="secret-type" className="mt-1 flex h-9 w-full rounded-md border bg-transparent px-3 text-sm" value={kind} onChange={event => setKind(event.target.value as SecretKind)}><option value="generic">Job secret</option><option value="registry">Registry credential</option></select></div>
      {kind === "registry" && <><div><Label htmlFor="registry-server">Registry server</Label><Input id="registry-server" value={registryServer} onChange={event => setRegistryServer(event.target.value)} placeholder="registry.example.com" /></div><div><Label htmlFor="registry-username">Username</Label><Input id="registry-username" autoComplete="username" value={registryUsername} onChange={event => setRegistryUsername(event.target.value)} /></div></>}
      <div><Label htmlFor="secret-value">{kind === "registry" ? "Password" : "Value"}</Label><Input id="secret-value" type="password" autoComplete="new-password" value={value} onChange={event => setValue(event.target.value)} /></div>
    </div><DialogFooter><Button variant="outline" onClick={close}>Cancel</Button><Button onClick={() => create.mutate()} disabled={!valid || create.isPending}>{create.isPending ? "Creating…" : "Create"}</Button></DialogFooter></DialogContent></Dialog></div>
    <div className="overflow-hidden rounded-md border"><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Type</TableHead><TableHead>Created</TableHead><TableHead /></TableRow></TableHeader><TableBody>{(query.data ?? []).map(secret => <TableRow key={secret.id}><TableCell className="flex items-center gap-2 font-medium"><KeyRound className="size-4 text-muted-foreground" />{secret.name}</TableCell><TableCell>{secret.kind === "registry" ? "Registry credential" : "Job secret"}</TableCell><TableCell>{relative(secret.created_at)}</TableCell><TableCell className="text-right"><Button variant="ghost" size="icon" aria-label={`Delete ${secret.name}`} disabled={remove.isPending} onClick={() => remove.mutate(secret.id)}><Trash2 className="size-4" /></Button></TableCell></TableRow>)}</TableBody></Table></div>
  </div>;
}
