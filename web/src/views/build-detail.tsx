import {useMutation,useQuery,useQueryClient} from "@tanstack/react-query";
import {Link,useParams} from "react-router-dom";
import {AlertTriangle,ArrowLeft,Check,Code2,LoaderCircle,PackageCheck,Play,Terminal,X} from "lucide-react";
import {toast} from "sonner";
import {api} from "@/api";
import {BuildStatusBadge} from "@/views/builds";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";

export function BuildDetail(){
  const {id=""}=useParams(),queryClient=useQueryClient();
  const build=useQuery({queryKey:["build",id],queryFn:()=>api.build(id),refetchInterval:5000});
  const hasPlan=build.data?.mode==="RAILPACK"&&build.data.status!=="CREATED";
  const plan=useQuery({queryKey:["build-plan",id],queryFn:()=>api.buildPlan(id),enabled:Boolean(hasPlan)});
  const logs=useQuery({queryKey:["build-logs",id],queryFn:()=>api.buildLogs(id),enabled:Boolean(build.data),refetchInterval:build.data&&["ANALYZING","BUILDING"].includes(build.data.status)?1000:false});
  const refresh=async()=>{await Promise.all([queryClient.invalidateQueries({queryKey:["build",id]}),queryClient.invalidateQueries({queryKey:["build-plan",id]})])};
  const confirm=useMutation({mutationFn:()=>api.confirmBuild(id),onSuccess:async()=>{toast.success("Build queued");await refresh()},onError:(cause:Error)=>toast.error(cause.message)});
  const cancel=useMutation({mutationFn:()=>api.cancelBuild(id),onSuccess:async()=>{toast.success("Build cancelled");await refresh()},onError:(cause:Error)=>toast.error(cause.message)});
  if(build.isPending)return <div className="grid min-h-72 place-items-center"><LoaderCircle className="size-5 animate-spin text-muted-foreground"/></div>;
  if(!build.data)return <div className="text-sm text-destructive">Build not found.</div>;
  const item=build.data;
  return <div className="space-y-5"><div className="flex items-start gap-3"><Button asChild variant="ghost" size="icon"><Link to="/builds"><ArrowLeft className="size-4"/><span className="sr-only">Back to builds</span></Link></Button><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h1 className="truncate text-xl font-semibold">{item.name}</h1><BuildStatusBadge status={item.status}/></div><div className="mt-1 font-mono text-xs text-muted-foreground">{item.id}</div></div></div>
    {item.status==="FAILED"&&<section className="flex gap-3 rounded-md border border-destructive/30 bg-destructive/5 p-4"><AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive"/><div><h2 className="text-sm font-medium">Build failed</h2><p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">{item.failure_reason}</p></div></section>}
    {plan.data&&<section className="overflow-hidden rounded-md border"><div className="flex items-center justify-between border-b px-4 py-3"><div className="flex items-center gap-2"><PackageCheck className="size-4"/><h2 className="text-sm font-medium">Detected configuration</h2>{plan.data.confirmed_at&&<Badge variant="secondary" className="bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"><Check className="size-3"/>Confirmed</Badge>}</div><span className="text-xs text-muted-foreground">Railpack {plan.data.railpack_version||"unknown"}</span></div>
      <dl className="grid sm:grid-cols-2 lg:grid-cols-4"><Detected label="Provider" value={plan.data.provider}/><Detected label="Runtime" value={plan.data.runtime}/><Detected label="Package manager" value={plan.data.package_manager}/><Detected label="Entrypoint" value={plan.data.entrypoint} mono/></dl>
      {!plan.data.confirmed_at&&item.status==="ANALYZING"&&<div className="flex justify-end gap-2 border-t bg-muted/20 px-4 py-3"><Button variant="outline" onClick={()=>cancel.mutate()} disabled={cancel.isPending}><X className="size-4"/>Cancel</Button><Button onClick={()=>confirm.mutate()} disabled={confirm.isPending}>{confirm.isPending?<LoaderCircle className="size-4 animate-spin"/>:<Check className="size-4"/>}Confirm build plan</Button></div>}
    </section>}
    {item.mode==="DOCKERFILE"&&item.status==="CREATED"&&<section className="flex items-center justify-between rounded-md border p-4"><div><h2 className="text-sm font-medium">Dockerfile build</h2><p className="text-xs text-muted-foreground">BuildKit will use the Dockerfile at the project root.</p></div><Button onClick={()=>confirm.mutate()} disabled={confirm.isPending}>{confirm.isPending?<LoaderCircle className="size-4 animate-spin"/>:<Play className="size-4"/>}Start build</Button></section>}
    {item.status==="BUILDING"&&<div className="flex justify-end"><Button variant="outline" onClick={()=>cancel.mutate()} disabled={cancel.isPending}><X className="size-4"/>Cancel build</Button></div>}
    {item.status==="SUCCEEDED"&&item.oci_digest&&<section className="rounded-md border border-emerald-500/30 bg-emerald-500/5 p-4"><div className="flex items-center gap-2 text-sm font-medium"><Check className="size-4 text-emerald-600"/>OCI build completed</div><div className="mt-2 break-all font-mono text-xs text-muted-foreground">{item.oci_digest}</div></section>}
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]"><section className="overflow-hidden rounded-md border"><div className="flex items-center gap-2 border-b px-4 py-3"><Terminal className="size-4"/><h2 className="text-sm font-medium">Build log</h2></div><pre className="max-h-[420px] overflow-auto whitespace-pre-wrap bg-zinc-950 p-4 font-mono text-xs leading-5 text-zinc-100">{logs.data||"No build output."}</pre></section>
      <section className="rounded-md border"><div className="flex items-center gap-2 border-b px-4 py-3"><Code2 className="size-4"/><h2 className="text-sm font-medium">Source identity</h2></div><dl className="space-y-3 p-4 text-sm"><Metadata label="Mode" value={item.mode}/><Metadata label="Archive" value={item.source.filename}/><Metadata label="SHA-256" value={item.source.sha256} mono/><Metadata label="Created" value={new Date(item.created_at).toLocaleString()}/></dl></section></div>
  </div>;
}

function Detected({label,value,mono=false}:{label:string;value:string;mono?:boolean}){return <div className="min-w-0 border-b p-4 sm:border-r lg:border-b-0"><dt className="text-xs text-muted-foreground">{label}</dt><dd className={`${mono?"font-mono text-xs":"text-sm font-medium"} mt-1 break-words`}>{value||"Not reported"}</dd></div>}
function Metadata({label,value,mono=false}:{label:string;value:string;mono?:boolean}){return <div><dt className="text-xs text-muted-foreground">{label}</dt><dd className={`${mono?"font-mono text-[11px]":""} mt-0.5 break-all`}>{value}</dd></div>}
