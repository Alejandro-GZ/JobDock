import {useState} from "react";
import {List,RotateCcw,ZoomIn,ZoomOut} from "lucide-react";
import {Button} from "@/components/ui/button";
import type {PlotSeries} from "@/components/observation-plot";

export function PlotToolbar({label,series,colors,mixedUnits,legend="auto",onZoomIn,onZoomOut,onReset}:{label:string;series:PlotSeries[];colors:Map<string,string>;mixedUnits:boolean;legend?:"auto"|"hidden"|"open";onZoomIn:()=>void;onZoomOut:()=>void;onReset:()=>void}){
  const [legendOpen,setLegendOpen]=useState(legend==="open");
  return <div data-plot-toolbar className="pointer-events-none absolute bottom-1 left-1/2 z-20 flex -translate-x-1/2 items-center rounded-md border bg-card/90 p-0.5 opacity-0 shadow-sm backdrop-blur-sm transition-opacity group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100">
    <Button type="button" variant="ghost" size="icon" className="size-6" aria-label={`Zoom in ${label}`} onClick={onZoomIn}><ZoomIn className="size-3"/></Button>
    <Button type="button" variant="ghost" size="icon" className="size-6" aria-label={`Zoom out ${label}`} onClick={onZoomOut}><ZoomOut className="size-3"/></Button>
    <Button type="button" variant="ghost" size="icon" className="size-6" aria-label={`Reset zoom ${label}`} onClick={onReset}><RotateCcw className="size-3"/></Button>
    {legend!=="hidden"&&<><span className="mx-0.5 h-4 w-px bg-border" aria-hidden/><Button type="button" variant="ghost" size="icon" className="size-6" aria-label={`${label} legend`} aria-expanded={legendOpen} onClick={()=>setLegendOpen(value=>!value)}><List className="size-3"/></Button></>}
    {legendOpen&&<div className="absolute bottom-8 left-1/2 z-30 min-w-44 -translate-x-1/2 space-y-1 rounded-md border bg-popover p-2 text-xs shadow-lg">{series.map(item=><div key={item.id} className="flex items-center gap-2"><i className="size-2 rounded-full" style={{backgroundColor:colors.get(item.id)}}/><span>{item.title}</span><span className="ml-auto text-muted-foreground">{item.unit||"unitless"}</span></div>)}{mixedUnits&&<p className="border-t pt-1 text-amber-600">Independent Y scales</p>}</div>}
  </div>;
}
