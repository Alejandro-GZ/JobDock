import * as TabsPrimitive from "@radix-ui/react-tabs";import {cn} from "@/lib/utils";
export const Tabs=TabsPrimitive.Root;
export function TabsList({className,...p}:React.ComponentProps<typeof TabsPrimitive.List>){return <TabsPrimitive.List className={cn("inline-flex h-9 items-center rounded-md bg-muted p-1",className)} {...p}/>}
export function TabsTrigger({className,...p}:React.ComponentProps<typeof TabsPrimitive.Trigger>){return <TabsPrimitive.Trigger className={cn("inline-flex h-7 items-center rounded-sm px-3 text-sm font-medium text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-sm",className)} {...p}/>}
export function TabsContent({className,...p}:React.ComponentProps<typeof TabsPrimitive.Content>){return <TabsPrimitive.Content className={cn("mt-4 outline-none",className)} {...p}/>}
