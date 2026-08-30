import { useRef,useState } from "react";
import { useMutation,useQuery,useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Plus,Search } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api";
import { JobTable } from "@/components/job-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select,SelectContent,SelectItem,SelectTrigger,SelectValue } from "@/components/ui/select";
import type { JobTelemetrySummary } from "@/types";

const summaryPoints=24;

export function Jobs(){
  const [search,setSearch]=useState(""),[status,setStatus]=useState("all"),[page,setPage]=useState(0);
  const queryClient=useQueryClient();
  const jobs=useQuery({queryKey:["jobs"],queryFn:api.jobs,refetchInterval:5000});
  const nodes=useQuery({queryKey:["nodes"],queryFn:api.nodes,staleTime:5000});
  const filtered=(jobs.data??[]).filter(job=>(status==="all"||job.status===status)&&(`${job.spec.name} ${job.id} ${job.spec.image}`.toLowerCase().includes(search.toLowerCase())));
  const pageSize=25,pageCount=Math.max(1,Math.ceil(filtered.length/pageSize)),currentPage=Math.min(page,pageCount-1),pageJobs=filtered.slice(currentPage*pageSize,(currentPage+1)*pageSize);
  const visibleIDs=pageJobs.map(job=>job.id),visibleKey=visibleIDs.join(",");
  const summaryRef=useRef<{key:string;cursor:number;items:Map<string,JobTelemetrySummary>}>({key:"",cursor:0,items:new Map()});
  if(summaryRef.current.key!==visibleKey)summaryRef.current={key:visibleKey,cursor:0,items:new Map()};
  const summaries=useQuery({
    queryKey:["job-list-telemetry",visibleKey],enabled:visibleIDs.length>0,refetchInterval:5000,
    queryFn:async()=>{
      const state=summaryRef.current,response=await api.jobTelemetrySummaries(visibleIDs,state.cursor||undefined,summaryPoints);
      if(state.key!==visibleKey)return new Map<string,JobTelemetrySummary>();
      for(const incoming of response.items){
        const previous=state.items.get(incoming.job_id);
        const resources=previous&&previous.attempt_id===incoming.attempt_id?[...previous.resources,...incoming.resources].slice(-summaryPoints):incoming.resources.slice(-summaryPoints);
        state.items.set(incoming.job_id,{...incoming,resources});
      }
      state.cursor=Math.max(state.cursor,response.cursor);
      return new Map(state.items);
    }
  });
  const action=useMutation({mutationFn:({job,kind}:{job:string;kind:"stop"|"rerun"|"delete"})=>kind==="stop"?api.stopJob(job):kind==="rerun"?api.rerunJob(job):api.deleteJob(job),onSuccess:(_,variables)=>{toast.success(variables.kind==="rerun"?"Job queued for a new attempt":variables.kind==="stop"?"Stop requested":"Job deletion started");queryClient.invalidateQueries({queryKey:["jobs"]})},onError:(error:Error)=>toast.error(error.message)});
  const nodeNames=new Map((nodes.data??[]).map(node=>[node.id,node.name]));
  return <div className="space-y-4"><div className="flex items-center gap-2"><div className="relative min-w-0 max-w-md flex-1"><Search className="absolute left-3 top-2.5 size-4 text-muted-foreground"/><Input className="pl-9" placeholder="Search jobs" value={search} onChange={event=>{setSearch(event.target.value);setPage(0)}}/></div><Select value={status} onValueChange={value=>{setStatus(value);setPage(0)}}><SelectTrigger className="w-40"><SelectValue/></SelectTrigger><SelectContent>{["all","QUEUED","RUNNING","SUCCEEDED","FAILED","CANCELLED","LOST"].map(item=><SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select><Button asChild size="sm" className="ml-auto"><Link to="/jobs/new"><Plus className="size-4"/>New job</Link></Button></div><JobTable jobs={pageJobs} telemetry={summaries.data} nodeNames={nodeNames} onAction={(job,kind)=>action.mutate({job:job.id,kind})}/>{filtered.length>pageSize&&<div className="flex items-center justify-end gap-2 text-xs text-muted-foreground"><span>Page {currentPage+1} of {pageCount}</span><Button size="sm" variant="outline" disabled={currentPage===0} onClick={()=>setPage(value=>Math.max(0,value-1))}>Previous</Button><Button size="sm" variant="outline" disabled={currentPage>=pageCount-1} onClick={()=>setPage(value=>Math.min(pageCount-1,value+1))}>Next</Button></div>}</div>;
}
