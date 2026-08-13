import { useEffect, useMemo, useRef, useState, type PointerEvent, type WheelEvent } from "react";
import { Info, RotateCcw, ZoomIn, ZoomOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { numericSummary, zoomDomain, type SeriesPoint, type TimeRange } from "@/lib/series";

export type ChartMarker = { id: string; timestamp: number; label: string; step?: number; href?: string };

const fallbackWidth = 720, fallbackHeight = 230, left = 52, right = 14, top = 10, bottom = 27;
const ranges: TimeRange[] = ["1h", "6h", "24h", "7d", "all"];

export function TimeSeriesChart({ title, points, markers = [], color = "var(--primary)", format = compact, summary: suppliedSummary }: { title: string; points: SeriesPoint[]; markers?: ChartMarker[]; color?: string; format?: (value: number) => string; summary?: { last: number; min: number; max: number } }) {
  const plotRef = useRef<HTMLDivElement>(null), [plotSize, setPlotSize] = useState({ width: fallbackWidth, height: fallbackHeight });
  const [range, setRange] = useState<TimeRange>("all"), rangedPoints = useMemo(() => chartRangePoints(points, range), [points, range]);
  const fullDomain = useMemo<[number, number]>(() => rangedPoints.length ? [rangedPoints[0].timestamp, Math.max(rangedPoints[0].timestamp + 1000, rangedPoints[rangedPoints.length - 1].timestamp)] : [0, 1], [rangedPoints]);
  const [zoom, setZoom] = useState<[number, number] | null>(null), [hover, setHover] = useState<SeriesPoint | null>(null), [markerHover, setMarkerHover] = useState<ChartMarker | null>(null), [statisticsPinned, setStatisticsPinned] = useState(false), [statisticsHovered, setStatisticsHovered] = useState(false);
  useEffect(() => { setZoom(null); setHover(null); }, [fullDomain]);
  useEffect(() => {
    const element = plotRef.current;
    if (!element || typeof ResizeObserver === "undefined") return;
    const resize = () => {
      const bounds = element.getBoundingClientRect(), width = Math.max(280, Math.round(bounds.width)), height = Math.max(112, Math.round(bounds.height));
      setPlotSize(current => current.width === width && current.height === height ? current : { width, height });
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const { width, height } = plotSize;
  const domain = zoom ?? fullDomain, visible = rangedPoints.filter(point => point.timestamp >= domain[0] && point.timestamp <= domain[1]);
  const summary = range === "all" && suppliedSummary ? suppliedSummary : numericSummary(rangedPoints), values = visible.map(point => point.value);
  const minimum = values.length ? Math.min(...values) : 0, maximum = values.length ? Math.max(...values) : 1, span = Math.max(1e-9, maximum - minimum);
  const x = (timestamp: number) => left + (timestamp - domain[0]) / Math.max(1, domain[1] - domain[0]) * (width - left - right);
  const y = (value: number) => top + (maximum - value) / span * (height - top - bottom);
  const path = visible.map((point, index) => `${index ? "L" : "M"}${x(point.timestamp).toFixed(2)},${y(point.value).toFixed(2)}`).join(" ");
  const applyZoom = (factor: number, anchor = .5) => { const next = zoomDomain(domain, factor, anchor), fullWidth = fullDomain[1] - fullDomain[0]; if (factor > 1 || next[1] - next[0] >= fullWidth) setZoom(null); else setZoom([Math.max(fullDomain[0], next[0]), Math.min(fullDomain[1], next[1])]); };
  const onWheel = (event: WheelEvent<SVGSVGElement>) => { event.preventDefault(); const bounds = event.currentTarget.getBoundingClientRect(); applyZoom(event.deltaY < 0 ? .7 : 1.4, (event.clientX - bounds.left) / bounds.width); };
  const onPointerMove = (event: PointerEvent<SVGSVGElement>) => { if (!visible.length) return; const bounds = event.currentTarget.getBoundingClientRect(), timestamp = domain[0] + (event.clientX - bounds.left) / bounds.width * (domain[1] - domain[0]); setHover(visible.reduce((closest, point) => Math.abs(point.timestamp - timestamp) < Math.abs(closest.timestamp - timestamp) ? point : closest)); };
  const statisticLabel = summary ? `${title} statistics: last ${format(summary.last)}, min ${format(summary.min)}, max ${format(summary.max)}, ${rangedPoints.length} points` : `${title} statistics`;
  const statisticsVisible = summary && (statisticsPinned || statisticsHovered);
  return <section className="flex min-h-0 flex-col rounded-md border bg-card p-2.5">
    <div className="flex h-7 shrink-0 items-center gap-1.5"><div className="relative flex min-w-0 items-center gap-0.5"><h3 className="max-w-32 truncate text-sm font-medium">{title}</h3>{summary && <button type="button" aria-label={statisticLabel} aria-expanded={statisticsPinned} onClick={() => setStatisticsPinned(value => !value)} onMouseEnter={() => setStatisticsHovered(true)} onMouseLeave={() => setStatisticsHovered(false)} onFocus={() => setStatisticsHovered(true)} onBlur={() => setStatisticsHovered(false)} onKeyDown={event => { if (event.key === "Escape") setStatisticsPinned(false); }} className="rounded-sm p-1 text-muted-foreground outline-none hover:text-foreground focus:ring-2 focus:ring-ring"><Info className="size-3"/></button>}{statisticsVisible && <div role="status" className="absolute left-full top-6 z-30 grid min-w-36 grid-cols-2 gap-x-3 gap-y-0.5 rounded-md bg-foreground px-2.5 py-2 text-xs text-background shadow-lg"><span>Last</span><strong>{format(summary.last)}</strong><span>Min</span><strong>{format(summary.min)}</strong><span>Max</span><strong>{format(summary.max)}</strong><span>Points</span><strong>{points.length}</strong></div>}</div>
      <div className="mx-auto flex h-6 shrink-0 items-center rounded border bg-muted/30 p-px" aria-label={`${title} time range`}>{ranges.map(item => <button key={item} type="button" aria-pressed={range === item} onClick={() => { setRange(item); setZoom(null); }} className={cn("h-5 rounded-sm px-1.5 text-[10px] text-muted-foreground hover:text-foreground", range === item && "bg-background font-medium text-foreground shadow-sm")}>{item === "all" ? "All" : item}</button>)}</div>
      <div className="ml-auto flex shrink-0"><Button type="button" variant="ghost" size="icon" className="size-6" aria-label={`Zoom in ${title}`} onClick={() => applyZoom(.6)}><ZoomIn className="size-3"/></Button><Button type="button" variant="ghost" size="icon" className="size-6" aria-label={`Zoom out ${title}`} onClick={() => applyZoom(1.6)}><ZoomOut className="size-3"/></Button><Button type="button" variant="ghost" size="icon" className="size-6" aria-label={`Reset zoom ${title}`} onClick={() => setZoom(null)}><RotateCcw className="size-3"/></Button></div></div>
    {rangedPoints.length === 0 ? <div className="grid min-h-0 flex-1 place-items-center text-sm text-muted-foreground">No samples in this range</div> : <div ref={plotRef} className="relative min-h-0 flex-1 overflow-hidden"><svg role="img" aria-label={`${title} time series with ${rangedPoints.length} points`} width={width} height={height} viewBox={`0 0 ${width} ${height}`} className="block size-full touch-none" onWheel={onWheel} onPointerMove={onPointerMove} onPointerLeave={() => setHover(null)}>
      {[0, .5, 1].map(ratio => <g key={ratio}><line x1={left} x2={width - right} y1={top + ratio * (height - top - bottom)} y2={top + ratio * (height - top - bottom)} className="stroke-border" strokeDasharray="3 4"/><text x={left - 7} y={top + ratio * (height - top - bottom) + 4} textAnchor="end" className="fill-muted-foreground text-[10px]">{format(maximum - ratio * span)}</text></g>)}{markers.filter(marker => marker.timestamp >= domain[0] && marker.timestamp <= domain[1]).map(marker => <g key={marker.id} role="button" tabIndex={0} aria-label={`Checkpoint ${marker.label}`} className="cursor-pointer" onMouseEnter={() => setMarkerHover(marker)} onMouseLeave={() => setMarkerHover(null)} onFocus={() => setMarkerHover(marker)} onBlur={() => setMarkerHover(null)} onClick={() => marker.href && (window.location.href = marker.href)} onKeyDown={event => { if ((event.key === "Enter" || event.key === " ") && marker.href) window.location.href = marker.href; }}><line x1={x(marker.timestamp)} x2={x(marker.timestamp)} y1={top} y2={height-bottom} stroke="#f59e0b" strokeWidth="2" strokeDasharray="4 3"/><circle cx={x(marker.timestamp)} cy={top+5} r="4" fill="#f59e0b"/></g>)}<path d={path} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke"/>{hover && <><line x1={x(hover.timestamp)} x2={x(hover.timestamp)} y1={top} y2={height - bottom} className="stroke-muted-foreground" strokeDasharray="3 3"/><circle cx={x(hover.timestamp)} cy={y(hover.value)} r="4" fill={color}/></>}<text x={left} y={height - 5} className="fill-muted-foreground text-[10px]">{shortTime(domain[0])}</text><text x={width - right} y={height - 5} textAnchor="end" className="fill-muted-foreground text-[10px]">{shortTime(domain[1])}</text>
    </svg>{markerHover ? <div className="pointer-events-none absolute right-2 top-2 rounded border bg-popover px-2 py-1 text-xs shadow"><div className="font-medium">Checkpoint · {markerHover.label}</div><div className="text-muted-foreground">{new Date(markerHover.timestamp).toLocaleString()}{markerHover.step != null ? ` · step ${markerHover.step}` : ""}</div></div> : hover && <div className="pointer-events-none absolute right-2 top-2 rounded border bg-popover px-2 py-1 text-xs shadow"><div className="font-medium">{format(hover.value)}</div><div className="text-muted-foreground">{new Date(hover.timestamp).toLocaleString()}{hover.step != null ? ` · step ${hover.step}` : ""}{hover.sampleCount && hover.sampleCount > 1 ? ` · ${hover.sampleCount} samples` : ""}</div></div>}</div>}
  </section>;
}

function compact(value: number) { return new Intl.NumberFormat(undefined, { maximumFractionDigits: 3, notation: Math.abs(value) >= 10000 ? "compact" : "standard" }).format(value); }
function shortTime(timestamp: number) { return new Date(timestamp).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }); }
export function chartRangePoints(points: SeriesPoint[], range: TimeRange) { if (range === "all" || !points.length) return points; const duration = { "1h": 3600e3, "6h": 21600e3, "24h": 86400e3, "7d": 604800e3 }[range]; const cutoff = points[points.length - 1].timestamp - duration; return points.filter(point => point.timestamp >= cutoff); }
