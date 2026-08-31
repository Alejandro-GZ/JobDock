import {useState} from "react";
import {useMutation,useQuery} from "@tanstack/react-query";
import {ArrowLeft,FileArchive,LoaderCircle} from "lucide-react";
import {Link,useNavigate} from "react-router-dom";
import {toast} from "sonner";
import {api} from "@/api";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {validateBuildSource} from "@/lib/builds";

export function NewBuild(){
  const navigate=useNavigate(),[name,setName]=useState(""),[source,setSource]=useState<File|null>(null),[error,setError]=useState("");
  const capabilities=useQuery({queryKey:["capabilities"],queryFn:api.capabilities});
  const create=useMutation({mutationFn:()=>api.createBuild(name,source!),onSuccess:build=>{if(build.status==="FAILED")toast.error("Project analysis failed");else toast.success("Project analyzed");navigate(`/builds/${build.id}`)},onError:(cause:Error)=>toast.error(cause.message)});
  const submit=(event:React.FormEvent)=>{event.preventDefault();const issue=validateBuildSource(name,source);setError(issue);if(!issue)create.mutate()};
  if(capabilities.data?.source_builds.enabled===false)return <div className="mx-auto max-w-2xl space-y-4"><Button asChild variant="ghost" size="sm"><Link to="/builds"><ArrowLeft className="size-4"/>Back to builds</Link></Button><div className="rounded-md border border-amber-500/30 bg-amber-500/5 p-6"><h1 className="font-semibold">Source builds are unavailable</h1><p className="mt-2 text-sm text-muted-foreground">{capabilities.data.source_builds.reason} You can still create jobs from prebuilt OCI images.</p><Button asChild className="mt-4"><Link to="/jobs/new">Create OCI job</Link></Button></div></div>;
  return <div className="mx-auto max-w-2xl space-y-6"><div className="flex items-center gap-3"><Button asChild variant="ghost" size="icon"><Link to="/builds"><ArrowLeft className="size-4"/><span className="sr-only">Back to builds</span></Link></Button><h1 className="text-xl font-semibold">Analyze project</h1></div>
    <form onSubmit={submit} className="space-y-6 rounded-md border p-6"><div className="space-y-2"><Label htmlFor="build-name">Build name</Label><Input id="build-name" value={name} onChange={event=>setName(event.target.value)} placeholder="training-service" autoFocus/></div>
      <div className="space-y-2"><Label htmlFor="build-source">Project archive</Label><label htmlFor="build-source" className="flex cursor-pointer items-center gap-3 rounded-md border border-dashed p-4 transition-colors hover:bg-accent"><FileArchive className="size-5 text-muted-foreground"/><span className="min-w-0"><span className="block truncate text-sm font-medium">{source?.name??"Choose a project archive"}</span><span className="block text-xs text-muted-foreground">ZIP, TAR.GZ or TGZ · Railpack will detect the build configuration</span></span></label><Input id="build-source" className="sr-only" type="file" accept=".zip,.tar.gz,.tgz,application/zip,application/gzip" onChange={event=>setSource(event.target.files?.[0]??null)}/></div>
      {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}<div className="flex justify-end gap-2"><Button asChild variant="ghost"><Link to="/builds">Cancel</Link></Button><Button disabled={create.isPending}>{create.isPending?<LoaderCircle className="size-4 animate-spin"/>:<FileArchive className="size-4"/>}Analyze project</Button></div></form>
  </div>;
}
