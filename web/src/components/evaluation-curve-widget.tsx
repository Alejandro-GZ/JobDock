import {useQueries} from "@tanstack/react-query";
import {api} from "@/api";
import type {DashboardWidget} from "@/lib/dashboard-widgets";
import type {TablePage} from "@/types";

type CurveType="roc_curve"|"precision_recall_curve"|"calibration_curve";
const defaultColors=["#2563eb","#dc2626","#16a34a","#9333ea","#ea580c","#0891b2"];

export function EvaluationCurveWidget({jobID,attemptID,widget,colors=defaultColors}:{jobID:string;attemptID:string;widget:DashboardWidget;colors?:readonly string[]}){
  const type=widget.type as CurveType,sources=widget.sources.filter(source=>source.kind==="table").slice(0,8),queries=useQueries({queries:sources.map(source=>({queryKey:["job-table",jobID,attemptID,source.name,"curve"],queryFn:()=>api.table(jobID,attemptID,source.name,"limit=500"),staleTime:10_000}))}),tables=queries.flatMap(query=>query.data?[query.data]:[]),definition=curveDefinition(type);
  if(queries.some(query=>query.isLoading))return <div className="h-full animate-pulse rounded-md border bg-muted/20"/>;
  if(!tables.length)return <div role="status" className="grid h-full place-items-center rounded-md border text-xs text-muted-foreground">No compatible curve points.</div>;
  return <section className="relative h-full min-h-0 overflow-hidden rounded-md border bg-card" aria-label={definition.label}>
    <svg className="h-full w-full" viewBox="0 0 620 360" preserveAspectRatio="xMidYMid meet" role="img">
      <line x1="55" y1="315" x2="590" y2="315" className="stroke-border"/><line x1="55" y1="25" x2="55" y2="315" className="stroke-border"/>
      {type!=="precision_recall_curve"&&<line x1="55" y1="315" x2="590" y2="25" stroke="var(--muted-foreground)" strokeDasharray="5 5" opacity=".5"/>}
      {tables.map((table,index)=><Curve key={table.name} table={table} x={definition.x} y={definition.y} color={widget.appearance?.series?.[`table:${table.name}`]?.color??colors[index%colors.length]}/>) }
      <text x="322" y="350" textAnchor="middle" className="fill-muted-foreground text-[10px]">{definition.xLabel}</text><text x="15" y="170" textAnchor="middle" transform="rotate(-90 15 170)" className="fill-muted-foreground text-[10px]">{definition.yLabel}</text>
    </svg>
    <div className="absolute bottom-6 left-1/2 flex max-w-[85%] -translate-x-1/2 flex-wrap justify-center gap-2 rounded bg-background/85 px-2 py-1 text-[9px] backdrop-blur">{tables.map((table,index)=><span key={table.name} className="flex items-center gap-1"><i className="size-2 rounded-full" style={{background:widget.appearance?.series?.[`table:${table.name}`]?.color??colors[index%colors.length]}}/>{String(table.metadata?.model??table.name)}{summary(table)}</span>)}</div>
  </section>
}

function Curve({table,x,y,color}:{table:TablePage;x:string;y:string;color:string}){const points=table.items.flatMap(item=>typeof item.values[x]==="number"&&typeof item.values[y]==="number"?[{x:item.values[x] as number,y:item.values[y] as number,threshold:item.values.threshold}]:[]),path=points.map((point,index)=>`${index?"L":"M"} ${55+point.x*535} ${315-point.y*290}`).join(" ");return <g><path d={path} fill="none" stroke={color} strokeWidth="2"/>{points.map((point,index)=><circle key={index} cx={55+point.x*535} cy={315-point.y*290} r="3" fill={color}><title>{`${table.name}: x ${point.x.toFixed(3)}, y ${point.y.toFixed(3)}${typeof point.threshold==="number"?`, threshold ${point.threshold}`:""}`}</title></circle>)}</g>}
function curveDefinition(type:CurveType){return type==="roc_curve"?{label:"ROC curve",x:"fpr",y:"tpr",xLabel:"False-positive rate",yLabel:"True-positive rate"}:type==="precision_recall_curve"?{label:"Precision–Recall curve",x:"recall",y:"precision",xLabel:"Recall",yLabel:"Precision"}:{label:"Calibration curve",x:"predicted_probability",y:"observed_fraction",xLabel:"Predicted probability",yLabel:"Observed fraction"}}
function summary(table:TablePage){const values=table.metadata?.summary;if(!values||typeof values!=="object")return"";return` · ${Object.entries(values).map(([key,value])=>`${key} ${Number(value).toFixed(3)}`).join(" · ")}`}
