import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { Activity, Boxes, ChevronDown, KeyRound, LayoutDashboard, Menu, Moon, ScrollText, Server, Sun, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { useTheme } from "@/components/theme-provider";
import { cn } from "@/lib/utils";
import type { User } from "@/types";
const nav = [{ to: "/", label: "Overview", icon: LayoutDashboard }, { to: "/jobs", label: "Jobs", icon: Boxes }, { to: "/nodes", label: "Nodes", icon: Server }, { to: "/secrets", label: "Secrets", icon: KeyRound }, { to: "/users", label: "Users", icon: Users, admin: true }, { to: "/audit", label: "Audit", icon: ScrollText, admin: true }];
function Navigation({ user, close }: { user: User; close?: () => void }) { return <nav className="space-y-1 px-2">{nav.filter((item) => !item.admin || user.role === "admin").map(({ to, label, icon: Icon }) => <NavLink key={to} end={to === "/"} onClick={close} to={to} className={({ isActive }) => cn("flex h-9 items-center gap-3 rounded-md px-3 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", isActive && "bg-accent font-medium text-foreground")}><Icon className="size-4"/>{label}</NavLink>)}</nav>; }
export function AppShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [mobile, setMobile] = useState(false), { setTheme } = useTheme();
  return <div className="min-h-screen bg-background"><aside className="fixed inset-y-0 left-0 hidden w-56 border-r bg-muted/20 lg:block"><div className="flex h-14 items-center gap-2 px-5 font-semibold"><Activity className="size-5 text-primary"/>JobDock</div><Navigation user={user}/><div className="absolute inset-x-0 bottom-0 border-t p-3"><DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" className="w-full justify-between"><span className="truncate">{user.username}</span><ChevronDown className="size-4"/></Button></DropdownMenuTrigger><DropdownMenuContent align="end" className="w-48"><DropdownMenuItem onClick={() => setTheme("system")}><Activity className="size-4"/>System theme</DropdownMenuItem><DropdownMenuItem onClick={() => setTheme("light")}><Sun className="size-4"/>Light theme</DropdownMenuItem><DropdownMenuItem onClick={() => setTheme("dark")}><Moon className="size-4"/>Dark theme</DropdownMenuItem><Separator/><DropdownMenuItem onClick={onLogout}>Sign out</DropdownMenuItem></DropdownMenuContent></DropdownMenu></div></aside>
    <Dialog open={mobile} onOpenChange={setMobile}><DialogContent className="left-0 top-0 h-full max-w-64 translate-x-0 translate-y-0 rounded-none p-2"><DialogTitle className="flex h-12 items-center gap-2 px-3"><Activity className="size-5"/>JobDock</DialogTitle><Navigation user={user} close={() => setMobile(false)}/></DialogContent></Dialog>
    <Button variant="outline" size="icon" className="fixed left-3 top-3 z-40 rounded-full bg-background/90 shadow-md backdrop-blur lg:hidden" onClick={() => setMobile(true)}><Menu className="size-5"/><span className="sr-only">Open navigation</span></Button>
    <div className="lg:pl-56"><main className="mx-auto max-w-[1500px] p-4 pt-16 lg:p-6"><Outlet/></main></div></div>;
}
