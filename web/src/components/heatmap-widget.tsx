import {useMemo} from "react";
import type {DashboardWidgetAppearance} from "@/lib/dashboard-widgets";
import type {MatrixObservation} from "@/types";

export function HeatmapWidget({matrix,correlation=false,appearance}:{matrix:MatrixObservation;correlation?:boolean;appearance?:DashboardWidgetAppearance}){
  const rows=matrix.values.length,columns=matrix.values[0]?.length??0,rowLabels=axisLabels(matrix.row_labels,rows),columnLabels=axisLabels(matrix.column_labels,columns),numbers=matrix.values.flatMap(row=>row.filter((value):value is number=>value!=null)),manual=appearance?.heatmap_scale==="manual"&&appearance.heatmap_min!=null&&appearance.heatmap_max!=null;
  const [minimum,maximum]=manual?[appearance!.heatmap_min!,appearance!.heatmap_max!]:correlation?[-1,1]:extent(numbers),palette=appearance?.heatmap_palette??(correlation?"diverging":"sequential"),cell=Math.max(8,Math.min(32,512/Math.max(1,rows,columns))),left=rowLabels.length?Math.min(150,Math.max(46,...rowLabels.map(label=>label.length*6))):18,top=columnLabels.length?Math.min(130,Math.max(36,...columnLabels.map(label=>label.length*5))):18,width=left+columns*cell+14,height=top+rows*cell+28;
  const ticks=useMemo(()=>tickIndexes(rows,columns),[rows,columns]),resolution=matrix.resolution;
  return <section className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-md border bg-card p-2">
    <div className="mb-1 flex shrink-0 items-baseline gap-2 px-1"><h3 className="truncate text-xs font-medium">{matrix.name}</h3><span className="text-[9px] text-muted-foreground">{correlation?"Correlation · ":""}{rows}×{columns}{matrix.unit?` · ${matrix.unit}`:""}{resolution?.mode==="aggregated"?` · aggregated from ${resolution.original_rows}×${resolution.original_columns}`:" · full resolution"}</span></div>
    <div className="min-h-0 flex-1 overflow-auto"><svg role="img" aria-label={`${matrix.name} ${correlation?"correlation ":""}heatmap with ${rows} rows and ${columns} columns`} width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
      <defs><pattern id={`null-${matrix.id}`} width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><rect width="6" height="6" fill="var(--muted)"/><line x1="0" y1="0" x2="0" y2="6" stroke="var(--muted-foreground)" strokeOpacity=".3"/></pattern></defs>
      {ticks.columns.map(index=><text key={`column-${index}`} x={left+(index+.5)*cell} y={top-5} textAnchor="end" transform={`rotate(-45 ${left+(index+.5)*cell} ${top-5})`} className="fill-muted-foreground text-[8px]">{columnLabels[index]??index+1}</text>)}
      {ticks.rows.map(index=><text key={`row-${index}`} x={left-5} y={top+(index+.68)*cell} textAnchor="end" className="fill-muted-foreground text-[8px]">{rowLabels[index]??index+1}</text>)}
      {matrix.values.flatMap((row,rowIndex)=>row.map((value,columnIndex)=>{const rowLabel=rowLabels[rowIndex]??`Row ${rowIndex+1}`,columnLabel=columnLabels[columnIndex]??`Column ${columnIndex+1}`,display=value==null?"No value":`${format(value)}${matrix.unit?` ${matrix.unit}`:""}`;return <g key={`${rowIndex}-${columnIndex}`}><rect data-null={value==null||undefined} x={left+columnIndex*cell} y={top+rowIndex*cell} width={cell} height={cell} fill={value==null?`url(#null-${matrix.id})`:heatColor(value,minimum,maximum,palette)} stroke="var(--background)" strokeWidth={Math.min(1,cell*.08)}><title>{`${rowLabel} × ${columnLabel}: ${display}`}</title></rect>{value!=null&&rows<=12&&columns<=12&&<text x={left+(columnIndex+.5)*cell} y={top+(rowIndex+.62)*cell} textAnchor="middle" className="pointer-events-none fill-slate-950 text-[8px]">{format(value)}</text>}</g>}))}
      <text x={left} y={height-8} className="fill-muted-foreground text-[8px]">{format(minimum)}</text><text x={left+columns*cell} y={height-8} textAnchor="end" className="fill-muted-foreground text-[8px]">{format(maximum)}</text>
    </svg></div>
  </section>;
}

function axisLabels(labels:string[]|undefined,size:number){return labels?.length===size?labels:[]}
function extent(values:number[]):[number,number]{if(!values.length)return[0,1];const minimum=Math.min(...values),maximum=Math.max(...values);return minimum===maximum?[minimum,minimum+1]:[minimum,maximum]}
function tickIndexes(rows:number,columns:number){const select=(size:number)=>{const stride=Math.max(1,Math.ceil(size/12));const result:number[]=[];for(let index=0;index<size;index+=stride)result.push(index);return result};return{rows:select(rows),columns:select(columns)}}
function heatColor(value:number,minimum:number,maximum:number,palette:"sequential"|"diverging"){const ratio=Math.max(0,Math.min(1,(value-minimum)/Math.max(Number.EPSILON,maximum-minimum)));if(palette==="sequential")return rgb(interpolate([239,246,255],[29,78,216],ratio));return ratio<.5?rgb(interpolate([37,99,235],[248,250,252],ratio*2)):rgb(interpolate([248,250,252],[220,38,38],(ratio-.5)*2))}
function interpolate(start:number[],end:number[],ratio:number){return start.map((value,index)=>Math.round(value+(end[index]-value)*ratio))}
function rgb(values:number[]){return `rgb(${values.join(" ")})`}
function format(value:number){return new Intl.NumberFormat(undefined,{maximumFractionDigits:3}).format(value)}
