import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {ChevronLeft,ChevronRight} from "lucide-react";
import {api} from "@/api";
import {Button} from "@/components/ui/button";
import type {DashboardWidget} from "@/lib/dashboard-widgets";

export function FeatureImportanceWidget({jobID,attemptID,widget,onUpdate}:{jobID:string;attemptID:string;widget:DashboardWidget;onUpdate:(widget:DashboardWidget)=>void}){
  const [page,setPage]=useState(0);
  const source=widget.sources.find(item=>item.kind==="table"),limit=widget.feature_importance_top_n??20,absolute=widget.feature_importance_absolute??false;
  const queryString=`limit=${limit}&offset=${page*limit}&sort=value&order=desc&absolute=${absolute}`;
  const query=useQuery({queryKey:["job-table",jobID,attemptID,source?.name,"feature-importance",limit,page,absolute],queryFn:()=>api.table(jobID,attemptID,source!.name,queryString),enabled:!!source,staleTime:10_000}),table=query.data;
  if(query.isLoading)return <div className="h-full animate-pulse rounded-md border bg-muted/20"/>;
  const items=table?.items.flatMap(item=>typeof item.values.feature==="string"&&typeof item.values.value==="number"?[{feature:item.values.feature,value:item.values.value,method:String(item.values.method??table.metadata?.method??"")}]:[])??[];
  if(!table||!items.length)return <div role="status" className="grid h-full place-items-center rounded-md border text-xs text-muted-foreground">No feature importance values.</div>;
  const extent=Math.max(...items.map(item=>Math.abs(item.value)),1e-12),unit=table.columns.find(column=>column.name==="value")?.unit??"";
  return <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-md border bg-card p-2" aria-label="Feature importance">
    <div className="mb-1 flex items-center justify-between gap-2 text-[10px] text-muted-foreground"><span className="truncate">{String(table.metadata?.method??items[0].method)}{table.metadata?.model?` · ${String(table.metadata.model)}`:""}</span><div className="flex items-center gap-1"><select aria-label="Feature count" className="h-6 rounded border bg-background px-1" value={limit} onChange={event=>{setPage(0);onUpdate({...widget,feature_importance_top_n:Number(event.target.value)})}}>{[10,20,50,100].map(value=><option key={value} value={value}>Top {value}</option>)}</select><Button variant="ghost" className="h-6 px-2 text-[10px]" onClick={()=>{setPage(0);onUpdate({...widget,feature_importance_absolute:!absolute})}}>{absolute?"Magnitude":"Signed"}</Button></div></div>
    <div className="min-h-0 flex-1 space-y-1 overflow-hidden">{items.map(item=>{const width=Math.abs(item.value)/extent*48,left=item.value<0?50-width:50;return <div key={item.feature} className="grid min-h-4 grid-cols-[minmax(5rem,28%)_1fr_4rem] items-center gap-2 text-[10px]"><span className="truncate text-right" title={item.feature}>{item.feature}</span><div className="relative h-2.5 rounded-sm bg-muted"><i className="absolute inset-y-0 left-1/2 w-px bg-border"/><i className="absolute inset-y-0 rounded-sm" style={{left:`${left}%`,width:`${width}%`,background:item.value<0?"#dc2626":"#2563eb"}}/></div><span className="truncate tabular-nums" title={`${item.value} ${unit}`}>{format(item.value)}{unit?` ${unit}`:""}</span></div>})}</div>
    <div className="flex items-center justify-end gap-1 pt-1"><span className="mr-1 text-[9px] text-muted-foreground">{page*limit+1}–{Math.min((page+1)*limit,Number(table.total))} of {table.total}</span><Button variant="ghost" size="icon" className="size-6" aria-label="Previous importance page" disabled={page===0} onClick={()=>setPage(value=>Math.max(0,value-1))}><ChevronLeft className="size-3"/></Button><Button variant="ghost" size="icon" className="size-6" aria-label="Next importance page" disabled={(page+1)*limit>=Number(table.total)} onClick={()=>setPage(value=>value+1)}><ChevronRight className="size-3"/></Button></div>
  </section>;
}
function format(value:number){return new Intl.NumberFormat(undefined,{maximumFractionDigits:3,notation:Math.abs(value)>=10000?"compact":"standard"}).format(value)}
