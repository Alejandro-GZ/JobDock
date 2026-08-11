import { useEffect, useMemo, useState, type PointerEvent, type WheelEvent } from "react";
import { RotateCcw, ZoomIn, ZoomOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import { numericSummary, zoomDomain, type SeriesPoint } from "@/lib/series";

const width = 720, height = 210, left = 52, right = 14, top = 16, bottom = 30;

export function TimeSeriesChart({ title, points, color = "var(--primary)", format = compact, summary: suppliedSummary }: { title: string; points: SeriesPoint[]; color?: string; format?: (value: number) => string; summary?: {last:number;min:number;max:number} }) {
  const fullDomain = useMemo<[number, number]>(() => points.length ? [points[0].timestamp, Math.max(points[0].timestamp + 1000, points[points.length - 1].timestamp)] : [0, 1], [points]);
  const [zoom, setZoom] = useState<[number, number] | null>(null), [hover, setHover] = useState<SeriesPoint | null>(null);
  useEffect(() => { setZoom(null); setHover(null); }, [fullDomain]);
  const domain = zoom ?? fullDomain;
  const visible = points.filter(point => point.timestamp >= domain[0] && point.timestamp <= domain[1]);
  const summary = suppliedSummary ?? numericSummary(points), values = visible.map(point => point.value);
  const minimum = values.length ? Math.min(...values) : 0, maximum = values.length ? Math.max(...values) : 1, span = Math.max(1e-9, maximum - minimum);
  const x = (timestamp: number) => left + (timestamp - domain[0]) / Math.max(1, domain[1] - domain[0]) * (width - left - right);
  const y = (value: number) => top + (maximum - value) / span * (height - top - bottom);
  const path = visible.map((point, index) => `${index ? "L" : "M"}${x(point.timestamp).toFixed(2)},${y(point.value).toFixed(2)}`).join(" ");
  const applyZoom = (factor: number, anchor = .5) => {
    const next = zoomDomain(domain, factor, anchor), fullWidth = fullDomain[1] - fullDomain[0];
    if (factor > 1 || next[1] - next[0] >= fullWidth) setZoom(null);
    else setZoom([Math.max(fullDomain[0], next[0]), Math.min(fullDomain[1], next[1])]);
  };
  const onWheel = (event: WheelEvent<SVGSVGElement>) => { event.preventDefault(); const bounds = event.currentTarget.getBoundingClientRect(); applyZoom(event.deltaY < 0 ? .7 : 1.4, (event.clientX - bounds.left) / bounds.width); };
  const onPointerMove = (event: PointerEvent<SVGSVGElement>) => {
    if (!visible.length) return;
    const bounds = event.currentTarget.getBoundingClientRect(), timestamp = domain[0] + (event.clientX - bounds.left) / bounds.width * (domain[1] - domain[0]);
    setHover(visible.reduce((closest, point) => Math.abs(point.timestamp - timestamp) < Math.abs(closest.timestamp - timestamp) ? point : closest));
  };
  return <section className="rounded-md border bg-card p-3">
    <div className="flex items-start justify-between gap-3"><div><h3 className="text-sm font-medium">{title}</h3>{summary && <div className="mt-1 flex gap-3 font-mono text-[11px] text-muted-foreground"><span>last {format(summary.last)}</span><span>min {format(summary.min)}</span><span>max {format(summary.max)}</span></div>}</div><div className="flex gap-1"><Button type="button" variant="ghost" size="icon" className="size-7" aria-label={`Zoom in ${title}`} onClick={() => applyZoom(.6)}><ZoomIn className="size-3.5"/></Button><Button type="button" variant="ghost" size="icon" className="size-7" aria-label={`Zoom out ${title}`} onClick={() => applyZoom(1.6)}><ZoomOut className="size-3.5"/></Button><Button type="button" variant="ghost" size="icon" className="size-7" aria-label={`Reset zoom ${title}`} onClick={() => setZoom(null)}><RotateCcw className="size-3.5"/></Button></div></div>
    {points.length === 0 ? <div className="flex h-[210px] items-center justify-center text-sm text-muted-foreground">No samples in this range</div> : <div className="relative mt-2"><svg role="img" aria-label={`${title} time series with ${points.length} points`} viewBox={`0 0 ${width} ${height}`} className="w-full touch-none" onWheel={onWheel} onPointerMove={onPointerMove} onPointerLeave={() => setHover(null)}>
      {[0,.5,1].map(ratio => <g key={ratio}><line x1={left} x2={width-right} y1={top+ratio*(height-top-bottom)} y2={top+ratio*(height-top-bottom)} className="stroke-border" strokeDasharray="3 4"/><text x={left-7} y={top+ratio*(height-top-bottom)+4} textAnchor="end" className="fill-muted-foreground text-[10px]">{format(maximum-ratio*span)}</text></g>)}
      <path d={path} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke"/>
      {hover && <><line x1={x(hover.timestamp)} x2={x(hover.timestamp)} y1={top} y2={height-bottom} className="stroke-muted-foreground" strokeDasharray="3 3"/><circle cx={x(hover.timestamp)} cy={y(hover.value)} r="4" fill={color}/></>}
      <text x={left} y={height-7} className="fill-muted-foreground text-[10px]">{shortTime(domain[0])}</text><text x={width-right} y={height-7} textAnchor="end" className="fill-muted-foreground text-[10px]">{shortTime(domain[1])}</text>
    </svg>{hover && <div className="pointer-events-none absolute right-2 top-2 rounded border bg-popover px-2 py-1 text-xs shadow"><div className="font-medium">{format(hover.value)}</div><div className="text-muted-foreground">{new Date(hover.timestamp).toLocaleString()}{hover.step != null ? ` · step ${hover.step}` : ""}{hover.sampleCount && hover.sampleCount > 1 ? ` · ${hover.sampleCount} samples` : ""}</div></div>}</div>}
  </section>;
}

function compact(value:number){return new Intl.NumberFormat(undefined,{maximumFractionDigits:3,notation:Math.abs(value)>=10000?"compact":"standard"}).format(value)}
function shortTime(timestamp:number){return new Date(timestamp).toLocaleString(undefined,{month:"short",day:"numeric",hour:"2-digit",minute:"2-digit"})}
