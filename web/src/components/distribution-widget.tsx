import { useEffect, useRef, useState } from "react";
import type { DashboardWidgetAppearance } from "@/lib/dashboard-widgets";
import type { DistributionObservation } from "@/types";
import {axisTitle,formatAxisValue,linearTicks,responsivePlotBox} from "@/lib/chart-geometry";

const defaultColors=["#2563eb","#16a34a","#dc2626","#9333ea","#ea580c","#0891b2"];

export function DistributionWidget({type,title,items,appearance,colors=defaultColors}:{type:"histogram"|"boxplot"|"violin";title?:string;items:DistributionObservation[];appearance?:DashboardWidgetAppearance;colors?:readonly string[]}){
  const host=useRef<HTMLElement>(null),[size,setSize]=useState({width:620,height:260});
  useEffect(()=>{const element=host.current;if(!element)return;const update=()=>{const box=element.getBoundingClientRect();if(box.width>0&&box.height>0)setSize({width:box.width,height:box.height})};update();const observer=new ResizeObserver(update);observer.observe(element);return()=>observer.disconnect()},[]);
  const min=Math.min(...items.map(item=>item.summary.min)),max=Math.max(...items.map(item=>item.summary.max)),span=max-min||1;
  const {left,right,top,bottom,width:plotWidth,height:plotHeight}=responsivePlotBox(size.width,size.height,{left:64,top:24,bottom:44}),plotBottom=top+plotHeight,x=(value:number)=>left+(value-min)/span*plotWidth,xTicks=linearTicks([min,max],Math.max(2,Math.min(6,Math.floor(plotWidth/90))));
  const laneHeight=plotHeight/Math.max(1,items.length),laneY=(index:number)=>top+(index+.5)*laneHeight,color=(item:DistributionObservation,index:number)=>appearance?.series?.[`distribution:${item.name}`]?.color??colors[index%colors.length];
  return <section ref={host} className="relative h-full min-h-0 overflow-hidden rounded-md border bg-card" aria-label={`${type} distribution`}>
    {title&&<h3 className="pointer-events-none absolute left-3 top-2 z-10 text-[10px] font-medium">{title}</h3>}
    <svg className="block size-full" width={size.width} height={size.height} viewBox={`0 0 ${size.width} ${size.height}`} role="img" aria-label={`${type} distribution with ${items.length} series`}>
      <line x1={left} y1={plotBottom} x2={size.width-right} y2={plotBottom} className="stroke-border"/>
      <line x1={left} y1={top} x2={left} y2={plotBottom} className="stroke-border"/>
      {xTicks.map(value=><g key={value}><line x1={x(value)} x2={x(value)} y1={plotBottom} y2={plotBottom+4} className="stroke-muted-foreground"/><text x={x(value)} y={plotBottom+14} textAnchor="middle" className="fill-muted-foreground text-[9px]">{formatAxisValue(value)}</text></g>)}
      <text x={left+plotWidth/2} y={size.height-4} textAnchor="middle" className="fill-muted-foreground text-[9px] font-medium">{axisTitle("Value",items.every(item=>item.unit===items[0].unit)?items[0].unit:undefined)}</text>
      {type==="histogram"?<text x="11" y={top+plotHeight/2} textAnchor="middle" transform={`rotate(-90 11 ${top+plotHeight/2})`} className="fill-muted-foreground text-[9px] font-medium">Count</text>:items.map((item,index)=><text key={`${item.name}:${item.group}`} x={left-7} y={laneY(index)+3} textAnchor="end" className="fill-muted-foreground text-[8px]">{item.group==="default"?item.name:item.group}</text>)}
      {type==="histogram"&&items.map((item,index)=>{const peak=Math.max(1,...items.flatMap(candidate=>candidate.bins.map(bin=>bin.count))),c=color(item,index);return <g key={`${item.name}:${item.group}`} opacity={.72}>{item.bins.map((bin,i)=>{const height=bin.count/peak*Math.max(1,plotHeight-8);return <rect key={i} x={x(bin.lower)} y={plotBottom-height} width={Math.max(1,x(bin.upper)-x(bin.lower)-1)} height={height} fill={c}/>})}</g>})}
      {type==="boxplot"&&items.map((item,index)=>{const y=laneY(index),half=Math.max(5,Math.min(17,laneHeight*.32)),s=item.summary,c=color(item,index);return <g key={`${item.name}:${item.group}`}><line x1={x(s.whisker_low)} x2={x(s.whisker_high)} y1={y} y2={y} stroke={c}/><line x1={x(s.whisker_low)} x2={x(s.whisker_low)} y1={y-half*.6} y2={y+half*.6} stroke={c}/><line x1={x(s.whisker_high)} x2={x(s.whisker_high)} y1={y-half*.6} y2={y+half*.6} stroke={c}/><rect x={x(s.q1)} y={y-half} width={Math.max(1,x(s.q3)-x(s.q1))} height={half*2} fill={c} fillOpacity=".22" stroke={c}/><line x1={x(s.median)} x2={x(s.median)} y1={y-half} y2={y+half} stroke={c} strokeWidth="2"/>{s.outliers.map((value,i)=><circle key={i} cx={x(value)} cy={y} r="2.5" fill={c}/>)}</g>})}
      {type==="violin"&&items.map((item,index)=>{const y=laneY(index),c=color(item,index),peak=Math.max(1e-12,...item.density.map(point=>point.density)),half=Math.max(5,Math.min(24,laneHeight*.38)),upper=item.density.map(point=>`${x(point.x)},${y-point.density/peak*half}`).join(" "),lower=[...item.density].reverse().map(point=>`${x(point.x)},${y+point.density/peak*half}`).join(" ");return <polygon key={`${item.name}:${item.group}`} points={`${upper} ${lower}`} fill={c} fillOpacity=".3" stroke={c}/>})}
    </svg>
    <div className="pointer-events-none absolute bottom-4 left-1/2 flex max-w-[85%] -translate-x-1/2 gap-3 rounded bg-background/80 px-2 py-1 text-[9px] backdrop-blur">{items.map((item,index)=><span key={`${item.name}:${item.group}`} className="flex items-center gap-1 whitespace-nowrap"><i className="size-2 rounded-full" style={{background:color(item,index)}}/>{item.name}{item.group!=="default"?` · ${item.group}`:""}</span>)}</div>
    {items.some(item=>item.scores&&Object.keys(item.scores).length)&&<div className="absolute right-2 top-2 rounded bg-background/80 px-2 py-1 text-[9px] text-muted-foreground">{items.flatMap(item=>Object.entries(item.scores??{}).map(([name,value])=>`${name} ${format(value)}`)).join(" · ")}</div>}
  </section>
}

function format(value:number){return formatAxisValue(value)}
