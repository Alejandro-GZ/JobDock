export type ChartBox={left:number;right:number;top:number;bottom:number;width:number;height:number};

export function linearTicks(domain:readonly[number,number],count=5){
  if(count<=1||domain[0]===domain[1])return[domain[0]];
  const span=domain[1]-domain[0];
  return Array.from({length:count},(_,index)=>domain[0]+span*index/(count-1));
}

export function formatAxisValue(value:number){
  return new Intl.NumberFormat(undefined,{maximumFractionDigits:3,notation:Math.abs(value)>=10_000?"compact":"standard"}).format(value);
}

export function axisTitle(label:string|undefined,unit:string|undefined,fallback="Value"){
  const name=label||fallback;
  return unit?`${name} (${unit})`:name;
}

export function responsivePlotBox(width:number,height:number,{left=56,right=18,top=16,bottom=42}:{left?:number;right?:number;top?:number;bottom?:number}={}):ChartBox{
  return{left,right,top,bottom,width:Math.max(1,width-left-right),height:Math.max(1,height-top-bottom)};
}
