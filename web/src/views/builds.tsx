import {useQuery} from "@tanstack/react-query";
import {Link} from "react-router-dom";
import {PackageSearch,Plus} from "lucide-react";
import {api} from "@/api";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Table,TableBody,TableCell,TableHead,TableHeader,TableRow} from "@/components/ui/table";
import type {BuildStatus} from "@/types";

const tone:Record<BuildStatus,string>={CREATED:"bg-muted text-muted-foreground",ANALYZING:"bg-blue-500/10 text-blue-600 dark:text-blue-400",BUILDING:"bg-amber-500/10 text-amber-700 dark:text-amber-400",SUCCEEDED:"bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",FAILED:"bg-red-500/10 text-red-700 dark:text-red-400",CANCELLED:"bg-muted text-muted-foreground"};
export function BuildStatusBadge({status}:{status:BuildStatus}){return <Badge variant="secondary" className={tone[status]}>{status}</Badge>}

export function Builds(){
  const builds=useQuery({queryKey:["builds"],queryFn:api.builds,refetchInterval:5000});
  const capabilities=useQuery({queryKey:["capabilities"],queryFn:api.capabilities});
  const enabled=capabilities.data?.source_builds.enabled!==false;
  return <div className="space-y-4">
    {!enabled&&<div className="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm text-muted-foreground">{capabilities.data?.source_builds.reason} Existing build history remains available.</div>}
    <div className="overflow-hidden rounded-md border"><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Status</TableHead><TableHead>Source</TableHead><TableHead>Created</TableHead><TableHead className="text-right">{enabled?<Button asChild size="sm"><Link to="/builds/new"><Plus className="size-4"/>Analyze project</Link></Button>:<Button size="sm" disabled><Plus className="size-4"/>Analyze project</Button>}</TableHead></TableRow></TableHeader><TableBody>{(builds.data??[]).map(build=><TableRow key={build.id} className="cursor-pointer"><TableCell><Link className="font-medium hover:underline" to={`/builds/${build.id}`}>{build.name}</Link><div className="font-mono text-xs text-muted-foreground">{build.id.slice(0,8)}</div></TableCell><TableCell><BuildStatusBadge status={build.status}/></TableCell><TableCell><div>{build.source.filename}</div><div className="text-xs text-muted-foreground">{formatBytes(build.source.size)}</div></TableCell><TableCell className="text-muted-foreground">{new Date(build.created_at).toLocaleString()}</TableCell><TableCell/></TableRow>)}</TableBody></Table>
      {!builds.isPending&&(builds.data?.length??0)===0&&<div className="grid place-items-center gap-2 py-14 text-center"><PackageSearch className="size-7 text-muted-foreground"/><div className="text-sm font-medium">No source builds yet</div>{enabled&&<Button asChild variant="outline" size="sm"><Link to="/builds/new">Analyze a project</Link></Button>}</div>}
    </div></div>;
}

function formatBytes(value:number){if(value<1024)return `${value} B`;const units=["KiB","MiB","GiB"];let amount=value/1024,index=0;while(amount>=1024&&index<units.length-1){amount/=1024;index++}return `${amount.toFixed(amount>=10?0:1)} ${units[index]}`}
