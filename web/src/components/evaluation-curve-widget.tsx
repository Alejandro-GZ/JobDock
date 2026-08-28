import {useQueries} from "@tanstack/react-query";
import {api} from "@/api";
import type {DashboardWidget} from "@/lib/dashboard-widgets";
import type {TablePage} from "@/types";
import {formatAxisValue,linearTicks,responsivePlotBox,type ChartBox} from "@/lib/chart-geometry";
import {useElementSize} from "@/lib/use-element-size";

type CurveType="roc_curve"|"precision_recall_curve"|"calibration_curve";
const defaultColors=["#2563eb","#dc2626","#16a34a","#9333ea","#ea580c","#0891b2"];

export function EvaluationCurveWidget({jobID,attemptID,widget,colors=defaultColors}:{jobID:string;attemptID:string;widget:DashboardWidget;colors?:readonly string[]}){
  const [host,size]=useElementSize<HTMLElement>({width:620,height:360});
  const type=widget.type as CurveType,sources=widget.sources.filter(source=>source.kind==="table").slice(0,8),queries=useQueries({queries:sources.map(source=>({queryKey:["job-table",jobID,attemptID,source.name,"curve"],queryFn:()=>api.table(jobID,attemptID,source.name,"limit=500"),staleTime:10_000}))}),tables=queries.flatMap(query=>query.data?[query.data]:[]),definition=curveDefinition(type);
  if(queries.some(query=>query.isLoading))return <div className="h-full animate-pulse rounded-md border bg-muted/20"/>;
  if(!tables.length)return <div role="status" className="grid h-full place-items-center rounded-md border text-xs text-muted-foreground">No compatible curve points.</div>;
  const box=responsivePlotBox(size.width,size.height,{left:58,right:18,top:18,bottom:42}),ticks=linearTicks([0,1],5),px=(value:number)=>box.left+value*box.width,py=(value:number)=>box.top+(1-value)*box.height;
  return <section ref={host} className="relative h-full min-h-0 overflow-hidden rounded-md border bg-card" aria-label={definition.label}>
    <svg className="block size-full" width={size.width} height={size.height} viewBox={`0 0 ${size.width} ${size.height}`} role="img">
      {ticks.map(value=><g key={value}><line x1={box.left} y1={py(value)} x2={box.left+box.width} y2={py(value)} className="stroke-border" strokeDasharray="3 4"/><text x={box.left-7} y={py(value)+3} textAnchor="end" className="fill-muted-foreground text-[9px]">{formatAxisValue(value)}</text><line x1={px(value)} y1={box.top+box.height} x2={px(value)} y2={box.top+box.height+4} className="stroke-muted-foreground"/><text x={px(value)} y={box.top+box.height+14} textAnchor="middle" className="fill-muted-foreground text-[9px]">{formatAxisValue(value)}</text></g>)}
      <line x1={box.left} y1={box.top+box.height} x2={box.left+box.width} y2={box.top+box.height} className="stroke-muted-foreground"/><line x1={box.left} y1={box.top} x2={box.left} y2={box.top+box.height} className="stroke-muted-foreground"/>
      {type!=="precision_recall_curve"&&<line x1={px(0)} y1={py(0)} x2={px(1)} y2={py(1)} stroke="var(--muted-foreground)" strokeDasharray="5 5" opacity=".5"/>}
      {tables.map((table,index)=><Curve key={table.name} table={table} x={definition.x} y={definition.y} box={box} color={widget.appearance?.series?.[`table:${table.name}`]?.color??colors[index%colors.length]}/>) }
      <text x={box.left+box.width/2} y={size.height-4} textAnchor="middle" className="fill-muted-foreground text-[9px] font-medium">{definition.xLabel}</text><text x="11" y={box.top+box.height/2} textAnchor="middle" transform={`rotate(-90 11 ${box.top+box.height/2})`} className="fill-muted-foreground text-[9px] font-medium">{definition.yLabel}</text>
    </svg>
    <div className="absolute bottom-6 left-1/2 flex max-w-[85%] -translate-x-1/2 flex-wrap justify-center gap-2 rounded bg-background/85 px-2 py-1 text-[9px] backdrop-blur">{tables.map((table,index)=><span key={table.name} className="flex items-center gap-1"><i className="size-2 rounded-full" style={{background:widget.appearance?.series?.[`table:${table.name}`]?.color??colors[index%colors.length]}}/>{String(table.metadata?.model??table.name)}{summary(table)}</span>)}</div>
  </section>
}

function Curve({table,x,y,color,box}:{table:TablePage;x:string;y:string;color:string;box:ChartBox}){const points=table.items.flatMap(item=>typeof item.values[x]==="number"&&typeof item.values[y]==="number"?[{x:item.values[x] as number,y:item.values[y] as number,threshold:item.values.threshold}]:[]),px=(value:number)=>box.left+value*box.width,py=(value:number)=>box.top+(1-value)*box.height,path=points.map((point,index)=>`${index?"L":"M"} ${px(point.x)} ${py(point.y)}`).join(" ");return <g><path d={path} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke"/>{points.map((point,index)=><circle key={index} cx={px(point.x)} cy={py(point.y)} r="3" fill={color}><title>{`${table.name}: x ${point.x.toFixed(3)}, y ${point.y.toFixed(3)}${typeof point.threshold==="number"?`, threshold ${point.threshold}`:""}`}</title></circle>)}</g>}
function curveDefinition(type:CurveType){return type==="roc_curve"?{label:"ROC curve",x:"fpr",y:"tpr",xLabel:"False-positive rate",yLabel:"True-positive rate"}:type==="precision_recall_curve"?{label:"Precision–Recall curve",x:"recall",y:"precision",xLabel:"Recall",yLabel:"Precision"}:{label:"Calibration curve",x:"predicted_probability",y:"observed_fraction",xLabel:"Predicted probability",yLabel:"Observed fraction"}}
function summary(table:TablePage){const values=table.metadata?.summary;if(!values||typeof values!=="object")return"";return` · ${Object.entries(values).map(([key,value])=>`${key} ${Number(value).toFixed(3)}`).join(" · ")}`}
