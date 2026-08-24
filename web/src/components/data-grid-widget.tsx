import {useMemo,useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {Columns3,ChevronLeft,ChevronRight} from "lucide-react";
import {api} from "@/api";
import {Button} from "@/components/ui/button";
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger} from "@/components/ui/dropdown-menu";
import {Input} from "@/components/ui/input";
import type {DashboardWidget} from "@/lib/dashboard-widgets";

export function DataGridWidget({jobID,attemptID,widget,onUpdate}:{jobID:string;attemptID:string;widget:DashboardWidget;onUpdate:(widget:DashboardWidget)=>void}){
  const source=widget.sources[0],pageSize=widget.table_page_size??50,[page,setPage]=useState(0),[filters,setFilters]=useState<Record<string,string>>({});
  const query=useMemo(()=>{const params=new URLSearchParams({limit:String(pageSize),offset:String(page*pageSize),order:widget.table_sort_order??"asc"});if(widget.table_sort_by)params.set("sort",widget.table_sort_by);for(const [key,value] of Object.entries(filters))if(value)params.append("filter",`${key}=${value}`);return params.toString()},[page,pageSize,widget.table_sort_by,widget.table_sort_order,filters]);
  const table=useQuery({queryKey:["job-table",jobID,attemptID,source?.name,query],queryFn:()=>api.table(jobID,attemptID,source!.name,query),enabled:!!source});
  if(table.isLoading)return <div className="h-full animate-pulse rounded-md border bg-muted/20"/>;
  if(table.isError||!table.data)return <div role="alert" className="grid h-full place-items-center rounded-md border text-xs text-destructive">Table data could not be loaded.</div>;
  const available=table.data.columns,selected=widget.table_columns?.length?available.filter(column=>widget.table_columns!.includes(column.name)):available;
  const toggleColumn=(name:string,checked:boolean)=>{const names=new Set(selected.map(column=>column.name));if(checked)names.add(name);else if(names.size>1)names.delete(name);onUpdate({...widget,table_columns:[...names]})};
  const sort=(name:string)=>onUpdate({...widget,table_sort_by:name,table_sort_order:widget.table_sort_by===name&&widget.table_sort_order!=="desc"?"desc":"asc"});
  return <section className="relative flex h-full min-h-0 flex-col overflow-hidden rounded-md border bg-card" aria-label={`${table.data.name} data grid`}>
    <div className="flex h-8 shrink-0 items-center gap-1 border-b px-2"><span className="truncate text-[10px] text-muted-foreground">{table.data.total.toLocaleString()} rows</span><DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="sm" className="ml-auto h-6 px-2 text-[10px]"><Columns3 className="mr-1 size-3"/>Columns</Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="max-h-64 overflow-y-auto">{available.map(column=><DropdownMenuItem key={column.name} onSelect={event=>{event.preventDefault();toggleColumn(column.name,!selected.some(item=>item.name===column.name))}}><input type="checkbox" readOnly tabIndex={-1} className="mr-2" checked={selected.some(item=>item.name===column.name)}/>{column.name}</DropdownMenuItem>)}</DropdownMenuContent></DropdownMenu></div>
    <div className="min-h-0 flex-1 overflow-auto"><table className="w-max min-w-full border-collapse text-[10px]"><thead className="sticky top-0 z-10 bg-card"><tr>{selected.map(column=><th key={column.name} className="min-w-28 border-b border-r p-1 text-left font-medium"><button type="button" className="flex w-full items-center gap-1" onClick={()=>sort(column.name)}>{column.name}{column.unit&&<span className="font-normal text-muted-foreground">({column.unit})</span>}{widget.table_sort_by===column.name&&(widget.table_sort_order==="desc"?" ↓":" ↑")}</button><Input aria-label={`Filter ${column.name}`} value={filters[column.name]??""} onChange={event=>{setPage(0);setFilters(current=>({...current,[column.name]:event.target.value}))}} className="mt-1 h-5 min-w-24 px-1 text-[9px]"/></th>)}</tr></thead><tbody>{table.data.items.map(row=><tr key={row.cursor} style={{contentVisibility:"auto",containIntrinsicSize:"24px"}}>{selected.map(column=><td key={column.name} title={formatCell(row.values[column.name])} className="max-w-80 truncate border-b border-r px-2 py-1.5">{formatCell(row.values[column.name])}</td>)}</tr>)}</tbody></table>{table.data.items.length===0&&<div className="grid h-24 place-items-center text-xs text-muted-foreground">No matching rows.</div>}</div>
    <div className="flex h-7 shrink-0 items-center justify-end gap-1 border-t px-2 text-[10px]"><span>Page {page+1}</span><Button aria-label="Previous page" size="icon" variant="ghost" className="size-5" disabled={page===0} onClick={()=>setPage(value=>value-1)}><ChevronLeft className="size-3"/></Button><Button aria-label="Next page" size="icon" variant="ghost" className="size-5" disabled={(page+1)*pageSize>=table.data.total} onClick={()=>setPage(value=>value+1)}><ChevronRight className="size-3"/></Button></div>
  </section>
}

function formatCell(value:unknown){if(value==null)return"—";if(typeof value==="object")return JSON.stringify(value);return String(value)}
