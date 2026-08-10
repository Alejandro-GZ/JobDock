import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) { return twMerge(clsx(inputs)); }

export function formatBytes(value:number) {
  if (!value) return "0 B";
  const units=["B","KiB","MiB","GiB","TiB"];
  const index=Math.min(Math.floor(Math.log(value)/Math.log(1024)),units.length-1);
  return `${(value/1024**index).toFixed(index>2?1:0)} ${units[index]}`;
}

export function relative(value:string) {
  const seconds=Math.max(0,Math.round((Date.now()-new Date(value).getTime())/1000));
  if(seconds<60)return `${seconds}s ago`;
  if(seconds<3600)return `${Math.floor(seconds/60)}m ago`;
  if(seconds<86400)return `${Math.floor(seconds/3600)}h ago`;
  return new Date(value).toLocaleDateString();
}
