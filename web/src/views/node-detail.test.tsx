// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { NodeDetail } from "@/views/node-detail";
import type { NodeDetail as NodeDetailData, User } from "@/types";

const detail: NodeDetailData = {
  node: { id: "node-1", name: "GPU worker", status: "ONLINE", agent_version: "0.1", protocol_version: 1, architecture: "amd64", docker_version: "29", cpu_total_millis: 8000, cpu_allocated_millis: 2000, memory_total_bytes: 16 * 1024 ** 3, memory_allocated_bytes: 4 * 1024 ** 3, workspace_total_bytes: 100 * 1024 ** 3, workspace_free_bytes: 80 * 1024 ** 3, labels: { zone: "lab" }, capabilities: ["host_inventory_v1"], cpu_packages: [{ id: "0", vendor: "GenuineIntel", model: "Test CPU", physical_cores: 4, logical_cpus: [0, 1, 2, 3, 4, 5, 6, 7], total_millis: 8000, allocated_millis: 2000 }], gpus: [{ uuid: "GPU-1", model: "RTX 4060", vram_bytes: 8 * 1024 ** 3, pci_bus_id: "0000:01:00.0", driver_version: "580", compute_capability: "8.9", utilization_basis_points: 6500, memory_used_bytes: 2 * 1024 ** 3, temperature_celsius: 58, allocated: true, allocated_job_id: "job-1" }], gpu_discovery: { status: "available" }, system: { hostname: "worker-1", operating_system: "Linux", kernel_version: "6.8", architecture: "amd64" }, runtime: { docker_version: "29", storage_driver: "overlay2", cgroup_version: "2" }, last_heartbeat: new Date().toISOString() },
  usage: { cpu_observed_millis: 1200, memory_observed_bytes: 2 * 1024 ** 3, gpu_memory_observed_bytes: 2 * 1024 ** 3, gpu_utilization_basis_points: 6500, latest_telemetry_at: new Date().toISOString(), observed_allocation_count: 1 },
  allocations: [{ job_id: "job-1", job_name: "Training", attempt_id: "attempt-1", status: "RUNNING", reserved_cpu_millis: 2000, reserved_memory_bytes: 4 * 1024 ** 3, cpu_package_id: "0", cpu_set: "0-3", gpu_uuids: ["GPU-1"], observed_cpu_millis: 1200, observed_memory_bytes: 2 * 1024 ** 3, telemetry_captured_at: new Date().toISOString(), telemetry_status: "fresh", can_open: true }],
};

vi.mock("@/api", () => ({ api: { node: vi.fn(async () => detail), setNode: vi.fn() } }));

afterEach(cleanup);

describe("NodeDetail", () => {
  it("renders operational inventory and attributed usage", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user: User = { id: "owner", username: "owner", role: "member", created_at: new Date().toISOString() };
    render(<QueryClientProvider client={client}><TooltipProvider><MemoryRouter initialEntries={["/nodes/node-1"]}><Routes><Route path="/nodes/:id" element={<NodeDetail user={user}/>}/></Routes></MemoryRouter></TooltipProvider></QueryClientProvider>);
    expect(await screen.findByText("GPU worker")).toBeTruthy();
    expect(screen.getByText("Active allocations (1)")).toBeTruthy();
    expect(screen.getByText("Training")).toBeTruthy();
    expect(screen.getByText("RTX 4060")).toBeTruthy();
    expect(screen.getByText("Test CPU")).toBeTruthy();
    expect(screen.getByText("worker-1")).toBeTruthy();
    expect(screen.getByText("overlay2")).toBeTruthy();
    expect(screen.getByRole("meter", { name: "CPU observed usage" }).getAttribute("aria-valuenow")).toBe("1200");
  });

  it("renders legacy nodes with null capabilities", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user: User = { id: "owner", username: "owner", role: "member", created_at: new Date().toISOString() };
    const legacy = structuredClone(detail);
    legacy.node.capabilities = null as unknown as string[];
    legacy.node.cpu_packages = [];
    legacy.node.workspace_total_bytes = 0;
    const apiModule = await import("@/api");
    vi.mocked(apiModule.api.node).mockResolvedValueOnce(legacy);
    render(<QueryClientProvider client={client}><TooltipProvider><MemoryRouter initialEntries={["/nodes/node-1"]}><Routes><Route path="/nodes/:id" element={<NodeDetail user={user}/>}/></Routes></MemoryRouter></TooltipProvider></QueryClientProvider>);
    expect(await screen.findByText("GPU worker")).toBeTruthy();
    expect(screen.getByText("Legacy agent; no capabilities reported")).toBeTruthy();
    expect(screen.getByText("Detailed CPU topology requires an agent with host_inventory_v1.")).toBeTruthy();
  });
});
