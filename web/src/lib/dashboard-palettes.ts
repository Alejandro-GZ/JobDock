import type {DashboardAppearance,DashboardPaletteRef,DashboardWidgetAppearance} from "@/lib/dashboard-widgets";

export type DashboardPalette={id:string;name:string;scope:"dashboard"|"widget";colors:readonly string[];surface?:{background:string;card:string;border:string;text:string}};

export const dashboardPalettes:readonly DashboardPalette[]=[
  {id:"jobdock",name:"JobDock",scope:"dashboard",colors:["#2563eb","#7c3aed","#0891b2","#16a34a","#f59e0b","#dc2626"],surface:{background:"#f8fafc",card:"#ffffff",border:"#cbd5e1",text:"#0f172a"}},
  {id:"midnight",name:"Midnight",scope:"dashboard",colors:["#60a5fa","#a78bfa","#22d3ee","#34d399","#fbbf24","#fb7185"],surface:{background:"#090f1d",card:"#111827",border:"#334155",text:"#f8fafc"}},
  {id:"accessible",name:"Accessible",scope:"dashboard",colors:["#0072b2","#e69f00","#009e73","#cc79a7","#d55e00","#56b4e9"]},
  {id:"categorical",name:"Categorical",scope:"widget",colors:["#2563eb","#7c3aed","#ea580c","#059669","#dc2626","#0891b2"]},
  {id:"cool",name:"Cool",scope:"widget",colors:["#1d4ed8","#4f46e5","#7c3aed","#0891b2","#0d9488"]},
  {id:"warm",name:"Warm",scope:"widget",colors:["#b91c1c","#ea580c","#d97706","#db2777","#ca8a04"]},
  {id:"diverging",name:"Diverging",scope:"widget",colors:["#2563eb","#93c5fd","#f8fafc","#fca5a5","#dc2626"]},
  {id:"sequential",name:"Sequential",scope:"widget",colors:["#eff6ff","#bfdbfe","#60a5fa","#2563eb","#1e3a8a"]},
  {id:"monochrome",name:"Monochrome",scope:"widget",colors:["#111827","#374151","#6b7280","#9ca3af","#d1d5db"]},
] as const;

export const quickColors=["#2563eb","#7c3aed","#0891b2","#16a34a","#f59e0b","#ea580c","#dc2626","#db2777","#64748b","#111827"] as const;
export function paletteByRef(ref?:DashboardPaletteRef){return ref?dashboardPalettes.find(item=>item.id===ref.id):undefined}
export function effectiveColors(dashboard?:DashboardAppearance,widget?:DashboardWidgetAppearance){return paletteByRef(widget?.palette)?.colors??paletteByRef(dashboard?.palette)?.colors??dashboardPalettes[3].colors}
