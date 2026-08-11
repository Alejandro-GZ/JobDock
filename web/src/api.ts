import type{AuditEvent,Job,JobAttempt,JobEvent,JobSpec,MetricSeriesResponse,Node,ResourceSeriesResponse,Secret,User}from "./types";
import { jobFormData } from "@/lib/job-inputs";
let csrfToken="";
export class APIError extends Error{constructor(public status:number,public code:string,message:string){super(message)}}
async function request<T>(path:string,options:RequestInit={}):Promise<T>{const headers=new Headers(options.headers);if(options.body&&!(options.body instanceof FormData))headers.set("Content-Type","application/json");if(options.method&&!['GET','HEAD'].includes(options.method)){headers.set("X-CSRF-Token",csrfToken);if(!headers.has("Idempotency-Key")&&(path==="/jobs"||path.endsWith("/stop")||path.endsWith("/rerun")||options.method==="DELETE"))headers.set("Idempotency-Key",idempotencyKey())}const response=await fetch(`/api/v1${path}`,{...options,headers,credentials:"same-origin"});if(!response.ok){const problem=await response.json().catch(()=>({}));throw new APIError(response.status,problem.code??"request_failed",problem.detail??response.statusText)}if(response.status===204)return undefined as T;return response.json() as Promise<T>}
export const api={
  async me(){const result=await request<{user:User;csrf_token:string}>("/auth/me");csrfToken=result.csrf_token;return result.user},
  async login(username:string,password:string){const result=await request<{user:User;csrf_token:string}>("/auth/login",{method:"POST",body:JSON.stringify({username,password})});csrfToken=result.csrf_token;return result.user},
  logout:()=>request<void>("/auth/logout",{method:"POST"}),
  jobs:async()=> (await request<{items:Job[]|null}>("/jobs")).items??[],job:(id:string)=>request<Job>(`/jobs/${id}`),
  createJob:(spec:JobSpec,inputs:File[]=[])=>request<Job>("/jobs",{method:"POST",body:inputs.length?jobFormData(spec,inputs):JSON.stringify(spec)}),stopJob:(id:string)=>request(`/jobs/${id}/stop`,{method:"POST"}),rerunJob:(id:string)=>request<Job>(`/jobs/${id}/rerun`,{method:"POST"}),deleteJob:(id:string)=>request(`/jobs/${id}`,{method:"DELETE"}),
  attempts:async(id:string)=>(await request<{items:JobAttempt[]|null}>(`/jobs/${id}/attempts`)).items??[],
  events:async(id:string,after=0,attemptId="")=>(await request<{items:JobEvent[]|null}>(`/jobs/${id}/events?after=${after}${attemptId?`&attempt_id=${encodeURIComponent(attemptId)}`:""}`)).items??[],
  metrics:(id:string,query:string)=>request<MetricSeriesResponse>(`/jobs/${id}/metrics?${query}`),
  resources:(id:string,query:string)=>request<ResourceSeriesResponse>(`/jobs/${id}/resources?${query}`),
  openSeriesStream:(id:string,attemptId:string,after:number)=>new EventSource(`/api/v1/jobs/${encodeURIComponent(id)}/series/stream?attempt_id=${encodeURIComponent(attemptId)}&after=${after}`),
  metricsCSV:(id:string,query:string)=>seriesCSV(id,"metrics",query),
  resourcesCSV:(id:string,query:string)=>seriesCSV(id,"resources",query),
  openLogStream:(id:string,attemptId:string,stream:"stdout"|"stderr")=>new EventSource(`/api/v1/jobs/${encodeURIComponent(id)}/logs/${stream}/tail?after=tail&attempt_id=${encodeURIComponent(attemptId)}`),
  nodes:async()=> (await request<{items:Node[]|null}>("/nodes")).items??[],enrollmentToken:()=>request<{token:string;expires_at:string}>("/nodes/enrollment-tokens",{method:"POST"}),setNode:(id:string,action:"drain"|"resume")=>request(`/nodes/${id}/${action}`,{method:"POST"}),updateNode:(id:string,name:string,labels:Record<string,string>)=>request(`/nodes/${id}`,{method:"PATCH",body:JSON.stringify({name,labels})}),
  secrets:async()=> (await request<{items:Secret[]|null}>("/secrets")).items??[],createSecret:(name:string,value:string,kind:string)=>request<Secret>("/secrets",{method:"POST",body:JSON.stringify({name,value,kind})}),deleteSecret:(id:string)=>request<void>(`/secrets/${id}`,{method:"DELETE"}),
  users:async()=> (await request<{items:User[]|null}>("/users")).items??[],createUser:(username:string,password:string,role:string)=>request<User>("/users",{method:"POST",body:JSON.stringify({username,password,role})}),
  audit:async()=> (await request<{items:AuditEvent[]|null}>("/audit")).items??[],
};
function idempotencyKey(){if(typeof crypto.randomUUID==="function")return crypto.randomUUID();const bytes=crypto.getRandomValues(new Uint8Array(24));return Array.from(bytes,v=>v.toString(16).padStart(2,"0")).join("")}
function seriesCSV(id:string,kind:"metrics"|"resources",query:string){const params=new URLSearchParams(query);params.set("format","csv");params.set("limit","10000");return `/api/v1/jobs/${encodeURIComponent(id)}/${kind}?${params}`}
