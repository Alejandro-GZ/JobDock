import * as P from "@radix-ui/react-select";
import {Check,ChevronDown,ChevronUp} from "lucide-react";
import {cn} from "@/lib/utils";

export const Select=P.Root;
export const SelectValue=P.Value;

export function SelectTrigger({className,children,...props}:React.ComponentProps<typeof P.Trigger>){
  return <P.Trigger className={cn("flex h-9 w-full items-center justify-between rounded-md border bg-background px-3 text-sm shadow-sm outline-none focus:ring-2 focus:ring-ring",className)} {...props}>{children}<P.Icon><ChevronDown className="size-4 text-muted-foreground"/></P.Icon></P.Trigger>;
}

export function SelectContent({className,children,...props}:React.ComponentProps<typeof P.Content>){
  return <P.Portal><P.Content className={cn("z-50 max-h-[min(24rem,var(--radix-select-content-available-height))] min-w-[8rem] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md",className)} position="popper" {...props}><P.ScrollUpButton className="grid h-6 place-items-center"><ChevronUp className="size-4"/></P.ScrollUpButton><P.Viewport className="max-h-[22rem] overflow-y-auto overscroll-contain p-1">{children}</P.Viewport><P.ScrollDownButton className="grid h-6 place-items-center"><ChevronDown className="size-4"/></P.ScrollDownButton></P.Content></P.Portal>;
}

export function SelectItem({className,children,...props}:React.ComponentProps<typeof P.Item>){
  return <P.Item className={cn("relative flex cursor-default select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none focus:bg-accent",className)} {...props}><span className="absolute left-2"><P.ItemIndicator><Check className="size-4"/></P.ItemIndicator></span><P.ItemText>{children}</P.ItemText></P.Item>;
}
