import {createContext,useContext,useEffect,useState,type ReactNode} from "react";
export type Theme="dark"|"light"|"system";
const ThemeContext=createContext<{theme:Theme;setTheme:(theme:Theme)=>void}>({theme:"system",setTheme:()=>{}});
export function ThemeProvider({children}:{children:ReactNode}){const[theme,setThemeState]=useState<Theme>(()=>(localStorage.getItem("jobdock-theme") as Theme)||"system");useEffect(()=>{const root=document.documentElement;root.classList.remove("light","dark");const resolved=theme==="system"?(matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light"):theme;root.classList.add(resolved);root.style.colorScheme=resolved},[theme]);const setTheme=(next:Theme)=>{localStorage.setItem("jobdock-theme",next);setThemeState(next)};return <ThemeContext.Provider value={{theme,setTheme}}>{children}</ThemeContext.Provider>}
export const useTheme=()=>useContext(ThemeContext);
