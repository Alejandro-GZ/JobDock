import {useEffect,useRef,useState} from "react";

export function useElementSize<T extends HTMLElement>(minimum={width:120,height:80}){
  const ref=useRef<T>(null),[size,setSize]=useState(minimum);
  useEffect(()=>{const element=ref.current;if(!element)return;const update=()=>{const box=element.getBoundingClientRect(),width=Math.max(minimum.width,Math.round(box.width)),height=Math.max(minimum.height,Math.round(box.height));setSize(current=>current.width===width&&current.height===height?current:{width,height})};update();if(typeof ResizeObserver==="undefined")return;const observer=new ResizeObserver(update);observer.observe(element);return()=>observer.disconnect()},[minimum.height,minimum.width]);
  return [ref,size] as const;
}
