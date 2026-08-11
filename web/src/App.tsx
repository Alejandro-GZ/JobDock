import { Navigate, Route, Routes } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import { AppShell } from "@/components/app-shell";
import { JobNotifications } from "@/components/job-notifications";
import { Dashboard } from "@/views/dashboard";
import { Jobs } from "@/views/jobs";
import { JobDetail } from "@/views/job-detail";
import { NewJob } from "@/views/new-job";
import { Nodes } from "@/views/nodes";
import { Secrets } from "@/views/secrets";
import { Users } from "@/views/users";
import { Audit } from "@/views/audit";
import { Login } from "@/views/login";
export function App() {
  const qc = useQueryClient(), me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
  if (me.isPending) return <div className="grid min-h-screen place-items-center text-sm text-muted-foreground">Loading JobDock…</div>;
  if (!me.data) return <Login onLogin={() => qc.invalidateQueries({ queryKey: ["me"] })}/>;
  const logout = async () => { await api.logout(); qc.clear(); };
  return <><JobNotifications/><Routes><Route element={<AppShell user={me.data} onLogout={logout}/>}><Route index element={<Dashboard user={me.data}/>}/><Route path="jobs" element={<Jobs/>}/><Route path="jobs/new" element={<NewJob/>}/><Route path="jobs/:id" element={<JobDetail user={me.data}/>}/><Route path="nodes" element={<Nodes user={me.data}/>}/><Route path="secrets" element={<Secrets/>}/>{me.data.role === "admin" && <><Route path="users" element={<Users/>}/><Route path="audit" element={<Audit/>}/></>}</Route><Route path="*" element={<Navigate to="/" replace/>}/></Routes></>;
}
