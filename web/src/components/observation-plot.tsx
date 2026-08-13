import type { ReactNode } from "react";
import type { ChartMarker } from "@/components/time-series-chart";
import type { SeriesPoint } from "@/lib/series";

const width = 720, height = 250, left = 50, right = 16, top = 16, bottom = 30;

export function ObservationPlot({ type, title, points, markers = [], actions }: { type: "barplot" | "scatterplot"; title: string; points: SeriesPoint[]; markers?: ChartMarker[]; actions?: ReactNode }) {
  const visible = points.slice(-80), values = visible.map(point => point.value);
  const minimum = values.length ? Math.min(...values) : 0, maximum = values.length ? Math.max(...values) : 1, span = Math.max(1e-9, maximum - minimum);
  const useStep = type === "scatterplot" && visible.length > 0 && visible.every(point => point.step != null);
  const xValues = visible.map((point, index) => useStep ? point.step! : type === "barplot" ? index : point.timestamp);
  const xMin = xValues.length ? Math.min(...xValues) : 0, xMax = xValues.length ? Math.max(...xValues) : 1, xSpan = Math.max(1, xMax - xMin);
  const x = (value: number) => left + (value - xMin) / xSpan * (width - left - right);
  const y = (value: number) => top + (maximum - value) / span * (height - top - bottom);
  return <section className="flex min-h-[320px] flex-col rounded-md border bg-card p-2.5">
    <header className="flex h-7 items-center"><div><h3 className="text-sm font-medium">{title}</h3><p className="text-[10px] text-muted-foreground">{type === "barplot" ? "Recent observations" : useStep ? "Step · value" : "Captured time · value"}</p></div><div className="ml-auto">{actions}</div></header>
    {visible.length === 0 ? <div className="grid flex-1 place-items-center text-sm text-muted-foreground">No samples available</div> : <svg role="img" aria-label={`${title} ${type} with ${visible.length} points`} viewBox={`0 0 ${width} ${height}`} className="min-h-0 w-full flex-1">
      {[0, .5, 1].map(ratio => <g key={ratio}><line x1={left} x2={width-right} y1={top+ratio*(height-top-bottom)} y2={top+ratio*(height-top-bottom)} className="stroke-border" strokeDasharray="3 4"/><text x={left-7} y={top+ratio*(height-top-bottom)+4} textAnchor="end" className="fill-muted-foreground text-[10px]">{compact(maximum-ratio*span)}</text></g>)}
      {type === "barplot" ? visible.map((point,index) => { const slot=(width-left-right)/visible.length,barWidth=Math.max(2,slot*.72),barHeight=Math.max(1,height-bottom-y(point.value)); return <rect key={`${point.timestamp}-${index}`} x={left+index*slot+(slot-barWidth)/2} y={y(point.value)} width={barWidth} height={barHeight} rx="2" className="fill-primary"><title>{`${compact(point.value)} · ${new Date(point.timestamp).toLocaleString()}`}</title></rect> }) : visible.map((point,index) => <circle key={`${point.timestamp}-${index}`} cx={x(xValues[index])} cy={y(point.value)} r="4" className="fill-primary stroke-background" strokeWidth="1.5"><title>{`${useStep?`Step ${point.step}`:new Date(point.timestamp).toLocaleString()} · ${compact(point.value)}`}</title></circle>)}
      {type === "scatterplot" && !useStep && markers.filter(marker => marker.timestamp >= Math.min(...visible.map(point=>point.timestamp)) && marker.timestamp <= Math.max(...visible.map(point=>point.timestamp))).map(marker => <line key={marker.id} x1={x(marker.timestamp)} x2={x(marker.timestamp)} y1={top} y2={height-bottom} stroke="#f59e0b" strokeWidth="2" strokeDasharray="4 3"><title>{`Checkpoint · ${marker.label}${marker.step!=null?` · step ${marker.step}`:""}`}</title></line>)}
      <text x={left} y={height-5} className="fill-muted-foreground text-[10px]">{useStep?`Step ${xMin}`:type==="barplot"?`${visible.length} samples`:new Date(xMin).toLocaleDateString()}</text><text x={width-right} y={height-5} textAnchor="end" className="fill-muted-foreground text-[10px]">{useStep?`Step ${xMax}`:type==="scatterplot"?new Date(xMax).toLocaleDateString():"Latest"}</text>
    </svg>}
  </section>;
}

function compact(value:number){return new Intl.NumberFormat(undefined,{maximumFractionDigits:3,notation:Math.abs(value)>=10000?"compact":"standard"}).format(value)}
