import * as React from "react";import {cn} from "@/lib/utils";
export function Table({className,...props}:React.ComponentProps<"table">){return <div className="w-full overflow-auto"><table className={cn("w-full caption-bottom text-sm",className)} {...props}/></div>}
export function TableHeader(p:React.ComponentProps<"thead">){return <thead className="border-b" {...p}/>};export function TableBody(p:React.ComponentProps<"tbody">){return <tbody className="[&_tr:last-child]:border-0" {...p}/>};
export function TableRow({className,...p}:React.ComponentProps<"tr">){return <tr className={cn("border-b transition-colors hover:bg-muted/45",className)} {...p}/>};
export function TableHead({className,...p}:React.ComponentProps<"th">){return <th className={cn("h-10 px-3 text-left align-middle text-xs font-medium text-muted-foreground",className)} {...p}/>};
export function TableCell({className,...p}:React.ComponentProps<"td">){return <td className={cn("px-3 py-2.5 align-middle",className)} {...p}/>}
