import { ArrowDownRight, ArrowUpRight, Minus } from "lucide-react";
import { useId } from "react";
import { cn } from "@/lib/utils";
import { numericSummary } from "@/lib/series";
import type { DashboardWidget, ScalarAggregation } from "@/lib/dashboard-widgets";
import type { NumericWidgetSource } from "@/components/observability-dashboard";
type ScalarState = "good" | "warning" | "critical" | "neutral";
export function ScalarSummaryWidget({ widget, source }: {
    widget: DashboardWidget;
    source: NumericWidgetSource;
}) {
    const summary = source.summary ?? numericSummary(source.points);
    if (!summary)
        return <section role="status" className="grid size-full place-items-center rounded-md border bg-card px-5 text-center text-sm text-muted-foreground">No scalar observations yet</section>;
    const aggregation = widget.scalar_aggregation ?? "last", value = scalarValue(summary, aggregation), state = thresholdState(value, widget.warning_value, widget.critical_value, widget.threshold_direction), label = widget.title || source.title, formatted = format(source, value), delta = widget.show_delta && aggregation === "last" && source.points.length > 1 ? value - source.points.at(-2)!.value : undefined;
    if (widget.type === "kpi")
        return <section className={cn("relative flex size-full min-h-0 flex-col justify-center overflow-hidden rounded-md border bg-card px-4 py-3", stateBorder(state))} aria-label={`${label}: ${formatted}`}><div className="truncate text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{label}</div><div className="mt-1 truncate font-mono text-[clamp(1.35rem,4vh,2.5rem)] font-semibold leading-none">{formatted}</div><div className="mt-2 flex items-center gap-2 text-[10px] text-muted-foreground"><span className={cn("rounded-full px-1.5 py-0.5 font-medium", stateChip(state))}>{stateLabel(state)}</span><span className="uppercase">{aggregation}</span>{delta != null && <Delta value={delta} format={source.format}/>}</div></section>;
    const domain = scalarDomain(widget, summary), percent = position(value, domain), target = widget.target_value == null ? undefined : position(widget.target_value, domain), warning = widget.warning_value == null ? undefined : position(widget.warning_value, domain), critical = widget.critical_value == null ? undefined : position(widget.critical_value, domain);
    const direction = widget.threshold_direction ?? "higher_is_worse";
    const color = widget.appearance?.accent_color, gradient = widget.appearance?.gradient;
    return widget.gauge_style === "bullet" ? <Bullet label={label} rawValue={value} value={formatted} unit={source.unit} percent={percent} target={target} warning={warning} critical={critical} domain={domain} direction={direction} state={state} color={color} gradient={gradient}/> : <Gauge label={label} rawValue={value} value={formatted} percent={percent} target={target} warning={warning} critical={critical} domain={domain} direction={direction} state={state} color={color} gradient={gradient}/>;
}
export function scalarValue(summary: {
    last: number;
    min: number;
    max: number;
    avg: number;
}, aggregation: ScalarAggregation) { return summary[aggregation]; }
export function thresholdState(value: number, warning?: number, critical?: number, direction: "higher_is_worse" | "lower_is_worse" = "higher_is_worse"): ScalarState { if (direction === "lower_is_worse") {
    if (critical != null && value <= critical)
        return "critical";
    if (warning != null && value <= warning)
        return "warning";
}
else {
    if (critical != null && value >= critical)
        return "critical";
    if (warning != null && value >= warning)
        return "warning";
} return warning == null && critical == null ? "neutral" : "good"; }
function Gauge({ label, rawValue, value, percent, target, warning, critical, domain, direction, state, color, gradient }: {
    label: string;
    rawValue: number;
    value: string;
    percent: number;
    target?: number;
    warning?: number;
    critical?: number;
    domain: [
        number,
        number
    ];
    direction: "higher_is_worse" | "lower_is_worse";
    state: ScalarState;
    color?: string;
    gradient?: Array<{ offset: number; color: string }>;
}) { const dash = percent * 1.8, angle = (target ?? 0) * 1.8 - 90, ranges = thresholdRanges(warning, critical, direction), gradientID = useId(); return <section role="meter" aria-label={`${label}: ${value}`} aria-valuemin={domain[0]} aria-valuemax={domain[1]} aria-valuenow={rawValue} className="relative size-full min-h-0 overflow-hidden rounded-md border bg-card"><svg viewBox="0 0 160 100" className="size-full">{gradient && <defs><linearGradient id={gradientID} x1="0" y1="0" x2="1" y2="0">{gradient.map(stop => <stop key={`${stop.offset}-${stop.color}`} offset={stop.offset} stopColor={stop.color}/>)}</linearGradient></defs>}<path d="M 25 82 A 55 55 0 1 1 135 82" pathLength="180" fill="none" stroke="currentColor" strokeWidth="12" strokeLinecap="round" className="text-muted/60"/><ArcRange range={ranges.warning} color="#f59e0b" opacity={.25}/><ArcRange range={ranges.critical} color="#ef4444" opacity={.28}/><path d="M 25 82 A 55 55 0 1 1 135 82" pathLength="180" fill="none" stroke={gradient ? `url(#${gradientID})` : color ?? "currentColor"} strokeWidth="7" strokeLinecap="round" strokeDasharray={`${dash} 180`} className={gradient || color ? undefined : stateStroke(state)}/>{target != null && <line aria-label="Target" x1="80" y1="20" x2="80" y2="31" stroke="currentColor" strokeWidth="2" transform={`rotate(${angle} 80 82)`}/>}<text x="80" y="65" textAnchor="middle" className="fill-foreground text-[14px] font-semibold">{value}</text><text x="80" y="79" textAnchor="middle" className="fill-muted-foreground text-[8px]">{label}</text><text x="18" y="96" className="fill-muted-foreground text-[7px]">{compact(domain[0])}</text><text x="142" y="96" textAnchor="end" className="fill-muted-foreground text-[7px]">{compact(domain[1])}</text></svg></section>; }
function Bullet({ label, rawValue, value, unit, percent, target, warning, critical, domain, direction, state, color, gradient }: {
    label: string;
    rawValue: number;
    value: string;
    unit: string;
    percent: number;
    target?: number;
    warning?: number;
    critical?: number;
    domain: [
        number,
        number
    ];
    direction: "higher_is_worse" | "lower_is_worse";
    state: ScalarState;
    color?: string;
    gradient?: Array<{ offset: number; color: string }>;
}) { const ranges = thresholdRanges(warning, critical, direction), fill = gradient ? `linear-gradient(90deg, ${gradient.map(stop => `${stop.color} ${stop.offset * 100}%`).join(", ")})` : color; return <section role="meter" aria-label={`${label}: ${value}`} aria-valuemin={domain[0]} aria-valuemax={domain[1]} aria-valuenow={rawValue} className="flex size-full min-h-0 flex-col justify-center overflow-hidden rounded-md border bg-card px-4 py-3"><div className="flex items-baseline justify-between gap-3"><span className="truncate text-xs font-medium">{label}</span><span className="shrink-0 font-mono text-sm font-semibold">{value}</span></div><div className="relative mt-3 h-5 overflow-hidden rounded bg-muted/60"><Range range={ranges.warning} className="bg-amber-400/20"/><Range range={ranges.critical} className="bg-red-500/20"/><div className={cn("absolute inset-y-1 left-0 rounded-r transition-[width]", fill ? undefined : stateFill(state))} style={{ width: `${percent}%`, background: fill }}/>{target != null && <span className="absolute inset-y-0 w-0.5 bg-foreground" style={{ left: `${target}%` }} aria-label="Target"/>}</div><div className="mt-1 flex justify-between font-mono text-[9px] text-muted-foreground"><span>{compact(domain[0])}</span><span>{unit}</span><span>{compact(domain[1])}</span></div></section>; }
function thresholdRanges(warning: number | undefined, critical: number | undefined, direction: "higher_is_worse" | "lower_is_worse") { return direction === "lower_is_worse" ? { warning: warning == null ? undefined : [critical ?? 0, warning] as [
        number,
        number
    ], critical: critical == null ? undefined : [0, critical] as [
        number,
        number
    ] } : { warning: warning == null ? undefined : [warning, critical ?? 100] as [
        number,
        number
    ], critical: critical == null ? undefined : [critical, 100] as [
        number,
        number
    ] }; }
function Range({ range, className }: {
    range?: [
        number,
        number
    ];
    className: string;
}) { if (!range)
    return null; return <span className={cn("absolute inset-y-0", className)} style={{ left: `${range[0]}%`, width: `${Math.max(0, range[1] - range[0])}%` }}/>; }
function ArcRange({ range, color, opacity }: {
    range?: [
        number,
        number
    ];
    color: string;
    opacity: number;
}) { if (!range)
    return null; return <path d="M 25 82 A 55 55 0 1 1 135 82" pathLength="180" fill="none" stroke={color} strokeOpacity={opacity} strokeWidth="12" strokeDasharray={`${Math.max(0, range[1] - range[0]) * 1.8} 180`} strokeDashoffset={-range[0] * 1.8}/>; }
function Delta({ value, format }: {
    value: number;
    format?: (value: number) => string;
}) { const Icon = value > 0 ? ArrowUpRight : value < 0 ? ArrowDownRight : Minus; return <span className="ml-auto inline-flex items-center gap-0.5" title="Change from the previous observation"><Icon className="size-3"/>{value > 0 ? "+" : ""}{format?.(value) ?? compact(value)}</span>; }
function scalarDomain(widget: DashboardWidget, summary: {
    min: number;
    max: number;
}): [
    number,
    number
] { const candidates = [summary.min, summary.max, widget.target_value, widget.warning_value, widget.critical_value].filter((value): value is number => value != null && Number.isFinite(value)), minimum = widget.domain_min ?? Math.min(0, ...candidates), legacyMaximum = widget.gauge_max_mode === "fixed" ? widget.gauge_max_value : undefined, maximum = widget.domain_max ?? legacyMaximum ?? Math.max(1, ...candidates); return minimum < maximum ? [minimum, maximum] : [minimum, minimum + 1]; }
function position(value: number, [minimum, maximum]: [
    number,
    number
]) { return Math.max(0, Math.min(100, (value - minimum) / (maximum - minimum) * 100)); }
function format(source: NumericWidgetSource, value: number) { const formatted = source.format?.(value) ?? compact(value); return source.format ? formatted : `${formatted}${source.unit && source.unit !== "unitless" ? ` ${source.unit}` : ""}`; }
function compact(value: number) { return new Intl.NumberFormat(undefined, { maximumFractionDigits: 3, notation: Math.abs(value) >= 10000 ? "compact" : "standard" }).format(value); }
function stateLabel(state: ScalarState) { return state === "good" ? "On target" : state === "warning" ? "Warning" : state === "critical" ? "Critical" : "No threshold"; }
function stateBorder(state: ScalarState) { return state === "critical" ? "border-red-500/50" : state === "warning" ? "border-amber-500/50" : state === "good" ? "border-emerald-500/40" : ""; }
function stateChip(state: ScalarState) { return state === "critical" ? "bg-red-500/10 text-red-600 dark:text-red-400" : state === "warning" ? "bg-amber-500/10 text-amber-700 dark:text-amber-300" : state === "good" ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" : "bg-muted"; }
function stateStroke(state: ScalarState) { return state === "critical" ? "text-red-500" : state === "warning" ? "text-amber-500" : state === "good" ? "text-emerald-500" : "text-primary"; }
function stateFill(state: ScalarState) { return state === "critical" ? "bg-red-500" : state === "warning" ? "bg-amber-500" : state === "good" ? "bg-emerald-500" : "bg-primary"; }
