import * as P from "@radix-ui/react-tooltip";import {cn} from "@/lib/utils";
export const TooltipProvider=P.Provider;export const Tooltip=P.Root;export const TooltipTrigger=P.Trigger;
export function TooltipContent({className,sideOffset=6,...p}:React.ComponentProps<typeof P.Content>){return <P.Portal><P.Content sideOffset={sideOffset} className={cn("z-50 rounded-md bg-foreground px-2.5 py-1.5 text-xs text-background shadow",className)} {...p}/></P.Portal>}
