import * as P from "@radix-ui/react-dropdown-menu";import {cn} from "@/lib/utils";
export const DropdownMenu=P.Root;export const DropdownMenuTrigger=P.Trigger;
export function DropdownMenuContent({className,sideOffset=6,...p}:React.ComponentProps<typeof P.Content>){return <P.Portal><P.Content sideOffset={sideOffset} className={cn("z-50 min-w-40 rounded-md border bg-popover p-1 shadow-md",className)} {...p}/></P.Portal>}
export function DropdownMenuItem({className,...p}:React.ComponentProps<typeof P.Item>){return <P.Item className={cn("flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none focus:bg-accent",className)} {...p}/>}
