// @vitest-environment jsdom

import { cleanup,render,screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach,describe,expect,it,vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { JobTable } from "@/components/job-table";
import type { Job,JobTelemetrySummary } from "@/types";

const job:Job={id:"11111111-1111-4111-8111-111111111111",owner_id:"22222222-2222-4222-8222-222222222222",spec:{name:"Training run",image:"example/train:1",command:["train"],environment:{},secret_refs:[],resources:{cpu_millis:2000,memory_bytes:4*1024**3,gpu:{count:0,min_vram_bytes:0}},labels:{},node_selector:{}},status:"RUNNING",desired_status:"RUNNING",observed_status:"RUNNING",attempt_id:"33333333-3333-4333-8333-333333333333",created_at:"2026-08-24T10:00:00Z",started_at:"2026-08-24T10:00:01Z",version:3};

function view(summary?:JobTelemetrySummary){return render(<MemoryRouter><TooltipProvider><JobTable jobs={[job]} telemetry={summary?new Map([[job.id,summary]]):undefined}/></TooltipProvider></MemoryRouter>)}
afterEach(cleanup);

describe("JobTable",()=>{
  it("uses an accessible status indicator and neutral GPU cells",()=>{view();expect(screen.getByRole("img",{name:"Status: RUNNING"})).toBeTruthy();expect(screen.getAllByText("N/A")).toHaveLength(2);expect(screen.queryByText(job.id)).toBeNull()});
  it("renders bounded resource summaries and SDK progress",()=>{view({job_id:job.id,attempt_id:job.attempt_id,progress:.42,resources:[{attempt_id:job.attempt_id!,captured_at:"2026-08-24T10:00:05Z",resolution_seconds:5,sample_count:1,cpu_millis:1000,memory_bytes:1024**3}]});expect(screen.getByRole("img",{name:"CPU usage: 50%"})).toBeTruthy();expect(screen.getByText("42%")).toBeTruthy()});
  it("keeps metadata and lifecycle-valid actions in the keyboard menu",async()=>{const onAction=vi.fn();render(<MemoryRouter><TooltipProvider><JobTable jobs={[job]} onAction={onAction} nodeNames={new Map([["node-1","GPU worker"]])}/></TooltipProvider></MemoryRouter>);await userEvent.click(screen.getByRole("button",{name:"Actions for Training run"}));expect(screen.getByText(job.id)).toBeTruthy();expect(screen.getByText("RUNNING")).toBeTruthy();expect(screen.getByText("Stop")).toBeTruthy();expect(screen.queryByText("Rerun")).toBeNull();expect(screen.queryByText("Delete")).toBeNull()});
});
