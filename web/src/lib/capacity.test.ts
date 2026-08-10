import { describe, expect, it } from "vitest";
import { calculateCapacity } from "./capacity";
import type { Node, Resources } from "@/types";

const resources: Resources = { cpu_millis: 1000, memory_bytes: 2_000, gpu: { count: 1, min_vram_bytes: 6_000 } };
const node = (overrides: Partial<Node>): Node => ({ id: "node-a", name: "Node A", status: "ONLINE", agent_version: "test", architecture: "amd64", docker_version: "test", cpu_total_millis: 8000, cpu_allocated_millis: 2000, memory_total_bytes: 16000, memory_allocated_bytes: 4000, workspace_free_bytes: 10000, labels: { zone: "lab" }, gpus: [{ uuid: "gpu-a", model: "RTX", vram_bytes: 8000, allocated: false }], gpu_discovery: { status: "available" }, last_heartbeat: new Date().toISOString(), ...overrides });

describe("calculateCapacity", () => {
  it("uses capacity from one compatible node", () => { const result = calculateCapacity([node({})], { zone: "lab" }, resources); expect(result.feasible).toHaveLength(1); expect(result.cpuMax).toBe(6000); expect(result.memoryMax).toBe(12000); expect(result.gpuMax).toBe(1); expect(result.vramMax).toBe(8000); });
  it("never adds GPUs across nodes", () => { const result = calculateCapacity([node({ id: "a" }), node({ id: "b" })], {}, { ...resources, gpu: { count: 2, min_vram_bytes: 0 } }); expect(result.feasible).toHaveLength(0); expect(result.gpuMax).toBe(1); });
  it("filters selectors before calculating limits", () => { const result = calculateCapacity([node({})], { zone: "remote" }, resources); expect(result.nodes).toHaveLength(0); expect(result.cpuMax).toBe(0); });
});
