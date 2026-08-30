import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Boxes, Eye, EyeOff, FileKey2, KeyRound, LoaderCircle, MoreHorizontal, Plus, Search, Server, ShieldCheck, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { relative } from "@/lib/utils";
import type { Secret } from "@/types";

type SecretKind = "generic" | "registry";
type SecretFilter = "all" | SecretKind;

const filters: Array<{ value: SecretFilter; label: string }> = [
  { value: "all", label: "All" },
  { value: "generic", label: "Job secrets" },
  { value: "registry", label: "Registry credentials" },
];

export function Secrets() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [kind, setKind] = useState<SecretKind>("generic");
  const [registryServer, setRegistryServer] = useState("");
  const [registryUsername, setRegistryUsername] = useState("");
  const [visible, setVisible] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<SecretFilter>("all");
  const [deleteTarget, setDeleteTarget] = useState<Secret>();
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ["secrets"], queryFn: api.secrets });
  const reset = () => { setName(""); setValue(""); setKind("generic"); setRegistryServer(""); setRegistryUsername(""); setVisible(false); setSubmitted(false); };
  const close = () => { setOpen(false); reset(); };
  const valid = Boolean(name.trim() && value && (kind === "generic" || (registryServer.trim() && registryUsername.trim())));
  const create = useMutation({
    mutationFn: () => api.createSecret(name.trim(), kind === "registry" ? JSON.stringify({ serveraddress: registryServer.trim(), username: registryUsername.trim(), password: value }) : value, kind),
    onSuccess: () => { toast.success("Secret created"); close(); queryClient.invalidateQueries({ queryKey: ["secrets"] }); },
    onError: (error: Error) => toast.error(error.message),
  });
  const remove = useMutation({
    mutationFn: api.deleteSecret,
    onSuccess: () => { toast.success("Secret deleted"); setDeleteTarget(undefined); queryClient.invalidateQueries({ queryKey: ["secrets"] }); },
    onError: (error: Error) => toast.error(error.message),
  });
  const items = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return (query.data ?? []).filter(secret => (filter === "all" || secret.kind === filter) && (!needle || secret.name.toLowerCase().includes(needle)));
  }, [filter, query.data, search]);
  const submit = () => { setSubmitted(true); if (valid) create.mutate(); };

  return <div className="space-y-4">
    <div className="flex items-center justify-between gap-4">
      <div><h1 className="text-xl font-semibold">Secrets</h1><p className="mt-0.5 text-xs text-muted-foreground">Encrypted credentials for workloads and private image pulls.</p></div>
      <Dialog open={open} onOpenChange={next => next ? setOpen(true) : close()}>
        <DialogTrigger asChild><Button size="sm"><Plus className="size-4" />New secret</Button></DialogTrigger>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader><DialogTitle>Create secret</DialogTitle><DialogDescription>Values are encrypted, write-only, and never returned by the API.</DialogDescription></DialogHeader>
          <div className="space-y-5">
            <fieldset><legend className="mb-2 text-sm font-medium">Secret type</legend><div className="grid grid-cols-2 gap-2">
              <TypeOption selected={kind === "generic"} title="Job secret" detail="Mount as a file or environment variable." icon={FileKey2} onClick={() => setKind("generic")} />
              <TypeOption selected={kind === "registry"} title="Registry credential" detail="Authenticate private OCI image pulls." icon={Boxes} onClick={() => setKind("registry")} />
            </div></fieldset>
            <div><Label htmlFor="secret-name">Name</Label><Input id="secret-name" value={name} onChange={event => setName(event.target.value)} placeholder={kind === "generic" ? "model-api-token" : "production-registry"} aria-invalid={submitted && !name.trim()} />{submitted && !name.trim() && <p role="alert" className="mt-1 text-xs text-destructive">Name is required.</p>}</div>
            {kind === "registry" && <div className="grid gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2"><Label htmlFor="registry-server">Registry server</Label><Input id="registry-server" value={registryServer} onChange={event => setRegistryServer(event.target.value)} placeholder="registry.example.com" aria-invalid={submitted && !registryServer.trim()} />{submitted && !registryServer.trim() && <p role="alert" className="mt-1 text-xs text-destructive">Registry server is required.</p>}</div>
              <div><Label htmlFor="registry-username">Username</Label><Input id="registry-username" autoComplete="username" value={registryUsername} onChange={event => setRegistryUsername(event.target.value)} aria-invalid={submitted && !registryUsername.trim()} />{submitted && !registryUsername.trim() && <p role="alert" className="mt-1 text-xs text-destructive">Username is required.</p>}</div>
              <SecretValue value={value} visible={visible} setValue={setValue} setVisible={setVisible} label="Password or token" invalid={submitted && !value} />
            </div>}
            {kind === "generic" && <SecretValue value={value} visible={visible} setValue={setValue} setVisible={setVisible} label="Value" invalid={submitted && !value} />}
            <div className="flex gap-3 rounded-md border bg-muted/25 p-3"><ShieldCheck className="mt-0.5 size-4 shrink-0 text-emerald-600"/><p className="text-xs leading-relaxed text-muted-foreground">{kind === "generic" ? "Choose file injection for the safest default. Environment injection is available when the workload requires it; both modes are redacted from persisted logs." : "This credential is sent only to Docker for image pulls. Job containers cannot read it."}</p></div>
          </div>
          <DialogFooter><Button variant="outline" onClick={close}>Cancel</Button><Button onClick={submit} disabled={create.isPending}>{create.isPending ? <><LoaderCircle className="size-4 animate-spin"/>Creating…</> : "Create secret"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>

    <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
      <div className="relative min-w-0 flex-1 sm:max-w-sm"><Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"/><Input aria-label="Search secrets" className="pl-9" value={search} onChange={event => setSearch(event.target.value)} placeholder="Search"/></div>
      <div className="flex rounded-md border bg-muted/20 p-0.5" aria-label="Secret type filter">{filters.map(option => <Button key={option.value} size="sm" variant={filter === option.value ? "secondary" : "ghost"} className="h-7 px-2.5 text-xs" aria-pressed={filter === option.value} onClick={() => setFilter(option.value)}>{option.label}{query.data && <span className="text-[10px] text-muted-foreground">{option.value === "all" ? query.data.length : query.data.filter(secret => secret.kind === option.value).length}</span>}</Button>)}</div>
    </div>

    {query.isPending ? <SecretSkeleton/> : query.isError ? <EmptyState icon={Server} title="Secrets could not be loaded" detail={query.error.message} action={<Button size="sm" variant="outline" onClick={() => query.refetch()}>Try again</Button>} /> : (query.data?.length ?? 0) === 0 ? <EmptyState icon={KeyRound} title="No secrets yet" detail="Create a job secret or registry credential to get started." action={<Button size="sm" onClick={() => setOpen(true)}><Plus className="size-4"/>New secret</Button>} /> : items.length === 0 ? <EmptyState icon={Search} title="No matching secrets" detail="Change the search or type filter." action={<Button size="sm" variant="outline" onClick={() => { setSearch(""); setFilter("all"); }}>Clear filters</Button>} /> : <div className="overflow-hidden rounded-md border"><Table>
      <TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Type</TableHead><TableHead>Used for</TableHead><TableHead>Created</TableHead><TableHead className="w-12"><span className="sr-only">Actions</span></TableHead></TableRow></TableHeader>
      <TableBody>{items.map(secret => { const registry = secret.kind === "registry"; const Icon = registry ? Boxes : FileKey2; return <TableRow key={secret.id}>
        <TableCell><div className="flex items-center gap-2.5"><span className="grid size-8 place-items-center rounded-md bg-muted"><Icon className="size-4 text-muted-foreground"/></span><div><p className="font-medium">{secret.name}</p><p className="font-mono text-[10px] text-muted-foreground">{secret.id.slice(0, 12)}</p></div></div></TableCell>
        <TableCell><Badge variant="outline">{registry ? "Registry credential" : "Job secret"}</Badge></TableCell>
        <TableCell><div className="flex items-center gap-2 text-sm">{registry ? <Server className="size-3.5 text-muted-foreground"/> : <KeyRound className="size-3.5 text-muted-foreground"/>}{registry ? "Image pulls" : "Job injection"}</div></TableCell>
        <TableCell><span title={new Date(secret.created_at).toLocaleString()}>{relative(secret.created_at)}</span></TableCell>
        <TableCell className="text-right"><DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" aria-label={`Actions for ${secret.name}`}><MoreHorizontal className="size-4"/></Button></DropdownMenuTrigger><DropdownMenuContent align="end"><DropdownMenuItem className="text-destructive focus:text-destructive" onSelect={() => setDeleteTarget(secret)}><Trash2 className="size-4"/>Delete</DropdownMenuItem></DropdownMenuContent></DropdownMenu></TableCell>
      </TableRow>; })}</TableBody>
    </Table></div>}

    <AlertDialog open={Boolean(deleteTarget)} onOpenChange={next => { if (!next && !remove.isPending) setDeleteTarget(undefined); }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete {deleteTarget?.name}?</AlertDialogTitle><AlertDialogDescription>This permanently removes the encrypted value. Existing job history remains intact, but future jobs and image pulls referencing this secret will fail.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={remove.isPending}>Cancel</AlertDialogCancel><AlertDialogAction disabled={remove.isPending} onClick={event => { event.preventDefault(); if (deleteTarget) remove.mutate(deleteTarget.id); }}>{remove.isPending ? "Deleting…" : "Delete secret"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>;
}

function TypeOption({ selected, title, detail, icon: Icon, onClick }: { selected: boolean; title: string; detail: string; icon: typeof KeyRound; onClick: () => void }) {
  return <button type="button" aria-pressed={selected} onClick={onClick} className={`rounded-md border p-3 text-left transition-colors ${selected ? "border-primary bg-primary/5 ring-1 ring-primary/30" : "hover:bg-muted/50"}`}><Icon className={`mb-2 size-4 ${selected ? "text-primary" : "text-muted-foreground"}`}/><span className="block text-sm font-medium">{title}</span><span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">{detail}</span></button>;
}

function SecretValue({ value, visible, setValue, setVisible, label, invalid }: { value: string; visible: boolean; setValue: (value: string) => void; setVisible: (visible: boolean) => void; label: string; invalid: boolean }) {
  return <div><Label htmlFor="secret-value">{label}</Label><div className="relative"><Input id="secret-value" type={visible ? "text" : "password"} autoComplete="new-password" value={value} onChange={event => setValue(event.target.value)} className="pr-10" aria-invalid={invalid}/><button type="button" className="absolute right-0 top-0 grid size-9 place-items-center text-muted-foreground hover:text-foreground" onClick={() => setVisible(!visible)} aria-label={visible ? "Hide secret value" : "Show secret value"}>{visible ? <EyeOff className="size-4"/> : <Eye className="size-4"/>}</button></div>{invalid && <p role="alert" className="mt-1 text-xs text-destructive">Value is required.</p>}</div>;
}

function EmptyState({ icon: Icon, title, detail, action }: { icon: typeof KeyRound; title: string; detail: string; action: React.ReactNode }) {
  return <div className="grid min-h-64 place-items-center rounded-md border border-dashed"><div className="flex max-w-sm flex-col items-center px-6 text-center"><span className="mb-3 grid size-10 place-items-center rounded-full bg-muted"><Icon className="size-5 text-muted-foreground"/></span><h2 className="text-sm font-medium">{title}</h2><p className="mt-1 text-xs text-muted-foreground">{detail}</p><div className="mt-4">{action}</div></div></div>;
}

function SecretSkeleton() {
  return <div className="space-y-1 rounded-md border p-2">{Array.from({ length: 4 }, (_, index) => <div key={index} className="flex items-center gap-3 px-2 py-2.5"><Skeleton className="size-8"/><div className="flex-1 space-y-1.5"><Skeleton className="h-3 w-36"/><Skeleton className="h-2.5 w-24"/></div><Skeleton className="h-5 w-28"/><Skeleton className="h-3 w-20"/></div>)}</div>;
}
