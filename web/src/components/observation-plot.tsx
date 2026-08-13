import type { ReactNode } from "react";
import { TriangleAlert } from "lucide-react";
import type { ChartMarker } from "@/components/time-series-chart";
import type { SeriesPoint } from "@/lib/series";

export type PlotSeries={id:string;title:string;unit:string;points:SeriesPoint[];color?:string};
const width=720,height=250,left=54,right=18,top=18,bottom=34,palette=["#3b82f6","#8b5cf6","#f59e0b","#10b981","#ef4444","#06b6d4"];

export function ObservationPlot({type,title,series,xAxis="time",markers=[],actions}:{type:"lineplot"|"barplot"|"scatterplot";title:string;series:PlotSeries[];xAxis?:"time"|"step";markers?:ChartMarker[];actions?:ReactNode}){
  const colors=new Map(series.map((item,index)=>[item.id,item.color??palette[index%palette.length]]));
  const units=[...new Set(series.map(item=>item.unit||"unitless"))],mixedUnits=units.length>1;
  const prepared=series.map(item=>({...item,points:item.points.filter(point=>xAxis==="time"||point.step!=null).slice(-80)}));
  const allPoints=prepared.flatMap(item=>item.points),xValue=(point:SeriesPoint)=>xAxis==="step"?point.step!:point.timestamp;
  const xValues=allPoints.map(xValue),xMin=xValues.length?Math.min(...xValues):0,xMax=xValues.length?Math.max(...xValues):1,xSpan=Math.max(1,xMax-xMin),x=(value:number)=>left+(value-xMin)/xSpan*(width-left-right);
  const scaleFor=(item:PlotSeries)=>{const comparable=mixedUnits?prepared.find(candidate=>candidate.id===item.id)!.points:allPoints,values=comparable.map(point=>point.value),min=values.length?Math.min(...values):0,max=values.length?Math.max(...values):1,span=Math.max(1e-9,max-min);return{min,max,y:(value:number)=>top+(max-value)/span*(height-top-bottom)}};
  const scatter=type==="scatterplot"?scatterPairs(prepared[0],prepared[1]):[];
  const scatterX=scatter.map(item=>item.x),scatterY=scatter.map(item=>item.y),scatterXMin=scatterX.length?Math.min(...scatterX):0,scatterXMax=scatterX.length?Math.max(...scatterX):1,scatterYMin=scatterY.length?Math.min(...scatterY):0,scatterYMax=scatterY.length?Math.max(...scatterY):1;
  const sx=(value:number)=>left+(value-scatterXMin)/Math.max(1e-9,scatterXMax-scatterXMin)*(width-left-right),sy=(value:number)=>top+(scatterYMax-value)/Math.max(1e-9,scatterYMax-scatterYMin)*(height-top-bottom);
  const empty=type==="scatterplot"?scatter.length===0:allPoints.length===0;
  return <section className="flex min-h-[320px] flex-col rounded-md border bg-card p-2.5">
    <header className="flex min-h-7 items-start"><div className="min-w-0"><h3 className="truncate text-sm font-medium">{title}</h3><div className="flex flex-wrap gap-x-2 text-[10px] text-muted-foreground">{series.map(item=><span key={item.id} className="flex items-center gap-1"><i className="size-2 rounded-full" style={{backgroundColor:colors.get(item.id)}}/>{item.title} · {item.unit||"unitless"}</span>)}</div></div><div className="ml-auto shrink-0">{actions}</div></header>
    {mixedUnits&&type!=="scatterplot"&&<p role="status" className="mt-1 flex items-center gap-1 text-[10px] text-amber-600"><TriangleAlert className="size-3"/>Different units use independent Y scales.</p>}
    {empty?<div className="grid flex-1 place-items-center text-sm text-muted-foreground">No compatible samples available</div>:<svg role="img" aria-label={`${title} ${type} with ${type==="scatterplot"?scatter.length:allPoints.length} points`} viewBox={`0 0 ${width} ${height}`} className="min-h-0 w-full flex-1">
      {[0,.5,1].map(ratio=><line key={ratio} x1={left} x2={width-right} y1={top+ratio*(height-top-bottom)} y2={top+ratio*(height-top-bottom)} className="stroke-border" strokeDasharray="3 4"/>)}
      {type==="lineplot"&&prepared.map(item=>{const scale=scaleFor(item),path=item.points.map((point,index)=>`${index?"L":"M"}${x(xValue(point)).toFixed(2)},${scale.y(point.value).toFixed(2)}`).join(" ");return <path key={item.id} d={path} fill="none" stroke={colors.get(item.id)} strokeWidth="2" vectorEffect="non-scaling-stroke"/>})}
      {type==="barplot"&&prepared.flatMap((item,seriesIndex)=>{const scale=scaleFor(item),slot=Math.max(3,(width-left-right)/Math.max(1,allPoints.length)),barWidth=Math.max(2,slot*.75);return item.points.map((point,index)=>{const center=x(xValue(point)),offset=(seriesIndex-(prepared.length-1)/2)*barWidth,barHeight=Math.max(1,height-bottom-scale.y(point.value));return <rect key={`${item.id}-${index}`} x={center+offset-barWidth/2} y={scale.y(point.value)} width={barWidth} height={barHeight} rx="1.5" fill={colors.get(item.id)}><title>{`${item.title}: ${compact(point.value)} ${item.unit} · ${axisLabel(point,xAxis)}`}</title></rect>})})}
      {type==="scatterplot"&&scatter.map((point,index)=><circle key={index} cx={sx(point.x)} cy={sy(point.y)} r="4" fill={colors.get(prepared[1]?.id)} className="stroke-background" strokeWidth="1.5"><title>{`${prepared[0]?.title}: ${compact(point.x)} ${prepared[0]?.unit} · ${prepared[1]?.title}: ${compact(point.y)} ${prepared[1]?.unit}${point.step!=null?` · step ${point.step}`:""}`}</title></circle>)}
      {type!=="scatterplot"&&markers.flatMap(marker=>{const value=xAxis==="step"?marker.step:marker.timestamp;return value==null||value<xMin||value>xMax?[]:[<line key={marker.id} x1={x(value)} x2={x(value)} y1={top} y2={height-bottom} stroke="#f59e0b" strokeWidth="2" strokeDasharray="4 3"><title>{`Checkpoint · ${marker.label}${marker.step!=null?` · step ${marker.step}`:""}`}</title></line>]})}
      {type==="scatterplot"&&markers.flatMap(marker=>{const point=scatter.find(item=>item.step!=null&&item.step===marker.step);return point?[<circle key={marker.id} cx={sx(point.x)} cy={sy(point.y)} r="7" fill="none" stroke="#f59e0b" strokeWidth="2"><title>{`Checkpoint · ${marker.label} · step ${marker.step}`}</title></circle>]:[]})}
      <text x={left} y={height-6} className="fill-muted-foreground text-[10px]">{type==="scatterplot"?`${prepared[0]?.title} · ${prepared[0]?.unit}`:xAxis==="step"?`Step ${xMin}`:new Date(xMin).toLocaleString()}</text><text x={width-right} y={height-6} textAnchor="end" className="fill-muted-foreground text-[10px]">{type==="scatterplot"?`${prepared[1]?.title} · ${prepared[1]?.unit}`:xAxis==="step"?`Step ${xMax}`:new Date(xMax).toLocaleString()}</text>
    </svg>}
  </section>
}

function scatterPairs(xSeries?:PlotSeries,ySeries?:PlotSeries){if(!xSeries||!ySeries)return[];const byStep=new Map(ySeries.points.flatMap(point=>point.step==null?[]:[[point.step,point] as const]));const matched=xSeries.points.flatMap(point=>point.step!=null&&byStep.has(point.step)?[{x:point.value,y:byStep.get(point.step)!.value,step:point.step}]:[]);if(matched.length)return matched.slice(-80);const count=Math.min(xSeries.points.length,ySeries.points.length,80),xStart=xSeries.points.length-count,yStart=ySeries.points.length-count;return Array.from({length:count},(_,index)=>({x:xSeries.points[xStart+index].value,y:ySeries.points[yStart+index].value,step:undefined}))}
function axisLabel(point:SeriesPoint,axis:"time"|"step"){return axis==="step"?`step ${point.step}`:new Date(point.timestamp).toLocaleString()}
function compact(value:number){return new Intl.NumberFormat(undefined,{maximumFractionDigits:3,notation:Math.abs(value)>=10000?"compact":"standard"}).format(value)}
