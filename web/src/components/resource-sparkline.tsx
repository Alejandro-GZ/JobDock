import { Tooltip,TooltipContent,TooltipTrigger } from "@/components/ui/tooltip";

export function ResourceSparkline({values,label,format,domain}:{values:(number|null|undefined)[];label:string;format:(value:number)=>string;domain?:[number,number]}){
  const finite=values.filter((value):value is number=>value!=null&&Number.isFinite(value));
  if(!finite.length)return <span className="text-xs text-muted-foreground">—</span>;
  const min=domain?.[0]??Math.min(...finite),max=domain?.[1]??Math.max(...finite),span=Math.max(max-min,Number.EPSILON);
  const points=finite.map((value,index)=>`${finite.length===1?50:index/(finite.length-1)*100},${30-(Math.max(min,Math.min(max,value))-min)/span*26}`).join(" ");
  const latest=finite.at(-1)??0;
  return <Tooltip><TooltipTrigger asChild><span className="block w-24" tabIndex={0} role="img" aria-label={`${label}: ${format(latest)}`}><svg viewBox="0 0 100 32" preserveAspectRatio="none" className="h-8 w-full overflow-visible"><path d="M0 30H100" className="stroke-border" strokeWidth="1"/><polyline points={points} fill="none" className="stroke-primary" strokeWidth="2" vectorEffect="non-scaling-stroke"/></svg></span></TooltipTrigger><TooltipContent><div className="space-y-0.5"><div className="font-medium">{label} · {format(latest)}</div><div className="opacity-75">Min {format(Math.min(...finite))} · Max {format(Math.max(...finite))} · {finite.length} points</div></div></TooltipContent></Tooltip>;
}
