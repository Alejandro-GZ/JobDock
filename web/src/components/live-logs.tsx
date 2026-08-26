import { memo, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { AlertCircle, Radio } from "lucide-react";
import { api } from "@/api";
import { decodeBase64Bytes } from "@/lib/live-logs";
import type {DashboardWidgetAppearance} from "@/lib/dashboard-widgets";

export type StreamName = "stdout" | "stderr";
type ConnectionState = "connecting" | "live" | "reconnecting";
type Chunk = { stream: StreamName; start_offset: number; next_offset: number; data: string };
type LogFragment = { id:number;stream:StreamName;text:string };
const visibleLimit=2_000_000;

export function LiveLogs({ jobId, attemptId, streams=["stdout","stderr"], embedded=false, actions, appearance }: { jobId:string;attemptId:string;streams?:StreamName[];embedded?:boolean;actions?:ReactNode;appearance?:DashboardWidgetAppearance }) {
  const [fragments,setFragments]=useState<LogFragment[]>([]),[truncated,setTruncated]=useState(false),sequence=useRef(0),output=useRef<HTMLPreElement>(null);
  useEffect(()=>{setFragments([]);setTruncated(false);sequence.current=0},[jobId,attemptId,streams.join(":")]);
  const append=useCallback((stream:StreamName,text:string)=>setFragments(current=>{
    let next=[...current,{id:sequence.current++,stream,text}],length=next.reduce((total,item)=>total+item.text.length,0);
    if(length<=visibleLimit)return next;setTruncated(true);
    while(next.length>1&&length>visibleLimit){length-=next[0].text.length;next=next.slice(1)}
    return next;
  }),[]);
  const wantsCombined=streams.includes("stdout")&&streams.includes("stderr"),combined=useCombinedLogStream(jobId,attemptId,wantsCombined,append),useFallback=wantsCombined&&combined.fallback;
  const stdout=useLogStream(jobId,attemptId,"stdout",streams.includes("stdout")&&(!wantsCombined||useFallback),append),stderr=useLogStream(jobId,attemptId,"stderr",streams.includes("stderr")&&(!wantsCombined||useFallback),append);
  useEffect(()=>{if(output.current)output.current.scrollTop=output.current.scrollHeight},[fragments]);
  const states=wantsCombined&&!useFallback?[combined.connection]:streams.map(stream=>stream==="stdout"?stdout:stderr),live=states.length>0&&states.every(state=>state==="live");
  const status=live?"Live":states.some(state=>state==="reconnecting")?"Reconnecting":"Connecting";
  const console=<div data-log-console className="relative flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border bg-zinc-950"><div data-log-toolbar className="pointer-events-none absolute inset-x-0 top-0 z-10 flex h-6 items-center border-b border-zinc-700/70 bg-zinc-900/95 px-2 text-[10px] text-zinc-200 shadow-sm backdrop-blur-sm"><span className="font-medium">Logs</span>{actions&&<span className="pointer-events-auto ml-1">{actions}</span>}<span tabIndex={0} className={live?"ml-auto flex items-center gap-1 text-emerald-400":"ml-auto flex items-center gap-1 text-amber-400"} title={live?"Receiving incremental log updates":"The stream will reconnect automatically"}>{live?<Radio className="size-3"/>:<AlertCircle className="size-3"/>}{status}</span></div>{truncated&&<p className="z-0 mt-6 border-b bg-amber-500/10 px-3 py-1.5 text-xs text-amber-300">Earlier output is hidden to keep the browser responsive. The complete log remains available in the ZIP.</p>}<pre ref={output} aria-label="Log output" className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap p-3 pt-8 font-mono text-xs text-zinc-100">{fragments.length?fragments.map(fragment=><LogChunk key={fragment.id} fragment={fragment}/>):"No output yet."}</pre></div>;
  const style={"--log-font-size":`${appearance?.font_size??12}px`,"--stdout-color":appearance?.series?.["log:stdout"]?.color??appearance?.stdout_color??"#f4f4f5","--stderr-color":appearance?.series?.["log:stderr"]?.color??appearance?.stderr_color??"#f87171","--log-background":appearance?.background_color??"#09090b","--log-wrap":appearance?.wrap_lines===false?"pre":"pre-wrap"} as import("react").CSSProperties;
  return embedded?<section style={style} className="jobdock-log-appearance flex h-full min-h-0 flex-col rounded-md bg-card">{console}</section>:<div style={style} className="jobdock-log-appearance h-[calc(100dvh-13rem)] min-h-[430px]">{console}</div>;
}

const LogChunk=memo(function LogChunk({fragment}:{fragment:LogFragment}){return <span data-log-stream={fragment.stream}>{fragment.text}</span>});

function useCombinedLogStream(jobId:string,attemptId:string,enabled:boolean,onChunk:(stream:StreamName,text:string)=>void){
  const [connection,setConnection]=useState<ConnectionState>("connecting"),[fallback,setFallback]=useState(false),decoders=useRef<Record<StreamName,TextDecoder>>({stdout:new TextDecoder(),stderr:new TextDecoder()});
  useEffect(()=>{if(!enabled)return;setConnection("connecting");setFallback(false);decoders.current={stdout:new TextDecoder(),stderr:new TextDecoder()};let opened=false;const source=api.openLogStream(jobId,attemptId,"combined");source.onopen=()=>{opened=true;setConnection("live")};source.onerror=()=>{if(!opened){source.close();setFallback(true)}else setConnection("reconnecting")};source.addEventListener("log",event=>{const chunk=JSON.parse((event as MessageEvent<string>).data) as Chunk,bytes=decodeBase64Bytes(chunk.data);onChunk(chunk.stream,decoders.current[chunk.stream].decode(bytes,{stream:true}));setConnection("live")});return()=>source.close()},[jobId,attemptId,enabled,onChunk]);
  return{connection,fallback};
}

function useLogStream(jobId:string,attemptId:string,stream:StreamName,enabled:boolean,onChunk:(stream:StreamName,text:string)=>void){
  const [connection,setConnection]=useState<ConnectionState>("connecting"),cursor=useRef<number|undefined>(undefined),decoder=useRef(new TextDecoder());
  useEffect(()=>{if(!enabled)return;setConnection("connecting");cursor.current=undefined;decoder.current=new TextDecoder();const source=api.openLogStream(jobId,attemptId,stream);source.onopen=()=>setConnection("live");source.onerror=()=>setConnection("reconnecting");source.addEventListener("log",event=>{const chunk=JSON.parse((event as MessageEvent<string>).data) as Chunk,bytes=decodeBase64Bytes(chunk.data),duplicate=cursor.current===undefined?0:Math.max(0,cursor.current-chunk.start_offset);if(duplicate>=bytes.length)return;onChunk(stream,decoder.current.decode(bytes.subarray(duplicate),{stream:true}));cursor.current=chunk.next_offset;setConnection("live")});return()=>source.close()},[jobId,attemptId,stream,enabled,onChunk]);
  return connection;
}
