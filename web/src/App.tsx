import { Navigate, Route, Routes } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import { AppShell } from "@/components/app-shell";
import { JobNotifications } from "@/components/job-notifications";
import { Dashboard } from "@/views/dashboard";
import { Jobs } from "@/views/jobs";
import { Builds } from "@/views/builds";
import { NewBuild } from "@/views/new-build";
import { BuildDetail } from "@/views/build-detail";
import { JobDetail } from "@/views/job-detail";
import { NewJob } from "@/views/new-job";
import { Nodes } from "@/views/nodes";
import { NodeDetail } from "@/views/node-detail";
import { Secrets } from "@/views/secrets";
import { Users } from "@/views/users";
import { Audit } from "@/views/audit";
import { Tokens } from "@/views/tokens";
import { Login } from "@/views/login";
import { Setup } from "@/views/setup";

export async function signOut(logout: () => Promise<void>, clear: () => void, reload: () => void = () => window.location.reload()) {
  try { await logout(); } finally { clear(); reload(); }
}

export function App() {
  const qc = useQueryClient(), me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false }), setup = useQuery({queryKey:["setup-status"],queryFn:api.setupStatus,retry:false});
  if (me.isPending || setup.isPending) return <div className="grid min-h-screen place-items-center text-sm text-muted-foreground">Loading JobDock…</div>;
  if (!me.data && setup.data?.required) return <Setup status={setup.data} onComplete={()=>{void qc.invalidateQueries({queryKey:["me"]});void qc.invalidateQueries({queryKey:["setup-status"]})}}/>;
  if (!me.data) return <Login onLogin={() => qc.invalidateQueries({ queryKey: ["me"] })}/>;
  const logout = () => signOut(api.logout, () => qc.clear());
  return <><JobNotifications/><Routes><Route element={<AppShell user={me.data} onLogout={logout}/>}><Route index element={<Dashboard user={me.data}/>}/><Route path="jobs" element={<Jobs/>}/><Route path="jobs/new" element={<NewJob/>}/><Route path="jobs/:id" element={<JobDetail user={me.data}/>}/><Route path="builds" element={<Builds/>}/><Route path="builds/new" element={<NewBuild/>}/><Route path="builds/:id" element={<BuildDetail/>}/><Route path="nodes" element={<Nodes user={me.data}/>}/><Route path="nodes/:id" element={<NodeDetail user={me.data}/>}/><Route path="secrets" element={<Secrets/>}/><Route path="tokens" element={<Tokens/>}/>{me.data.role === "admin" && <><Route path="users" element={<Users/>}/><Route path="audit" element={<Audit/>}/></>}</Route><Route path="*" element={<Navigate to="/" replace/>}/></Routes></>;
}
