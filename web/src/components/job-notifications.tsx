import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { JobUpdate } from "@/types";

const relevant = new Set(["ASSIGNED","RUNNING","SUCCEEDED","FAILED","CANCELLED","LOST"]);
export function JobNotifications() { const queryClient=useQueryClient(); const navigate=useNavigate();
  useEffect(()=>{const saved=sessionStorage.getItem("jobdock-event-cursor");const source=new EventSource(`/api/v1/jobs/stream?scope=mine&after=${saved??"latest"}`);const seen=new Set<string>();
    const listener=(event:MessageEvent)=>{const update=JSON.parse(event.data) as JobUpdate;sessionStorage.setItem("jobdock-event-cursor",String(update.cursor));const key=`${update.job_id}:${update.version}`;if(seen.has(key)||!relevant.has(update.status))return;seen.add(key);queryClient.invalidateQueries({queryKey:["jobs"]});toast(update.name,{description:`Job is now ${update.status.toLowerCase()}.`,action:{label:"Open",onClick:()=>navigate(`/jobs/${update.job_id}`)}})};
    source.addEventListener("job-status",listener);return()=>source.close();
  },[navigate,queryClient]); return null;
}
