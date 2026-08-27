export type MemoryUnit="bytes"|"KiB"|"MiB"|"GiB";

export const memoryUnitFactors:Record<MemoryUnit,number>={bytes:1,KiB:1024,MiB:1024**2,GiB:1024**3};

export function bytesToUnit(bytes:number,unit:MemoryUnit){return bytes/memoryUnitFactors[unit]}
export function unitToBytes(value:number,unit:MemoryUnit){
  if(!Number.isFinite(value)||value<0)return 0;
  return Math.round(value*memoryUnitFactors[unit]);
}
export function memoryInputValue(bytes:number,unit:MemoryUnit){
  const value=bytesToUnit(bytes,unit);
  return Number.isInteger(value)?String(value):String(Number(value.toFixed(6)));
}
