import type { Node, Resources } from "@/types";

export type Capacity = { nodes: Node[]; feasible: Node[]; cpuMax: number; memoryMax: number; gpuMax: number; vramMax: number; reference?: Node };

const freeCpu = (node: Node) => Math.max(0, node.cpu_total_millis - node.cpu_allocated_millis);
const freeMemory = (node: Node) => Math.max(0, node.memory_total_bytes - node.memory_allocated_bytes);
const freeGPUs = (node: Node, minVram = 0) => node.gpus.filter((gpu) => !gpu.allocated && gpu.vram_bytes >= minVram);
const labelsMatch = (node: Node, selectors: Record<string, string>) => Object.entries(selectors).every(([key, value]) => node.labels[key] === value);
const gpuFits = (node: Node, resources: Resources) => {
  const explicit=resources.gpu.uuids??[];
  return explicit.length>0?explicit.every(uuid=>node.gpus.some(gpu=>gpu.uuid===uuid&&!gpu.allocated)):resources.gpu.count===0||freeGPUs(node,resources.gpu.min_vram_bytes).length>=resources.gpu.count;
};

export function calculateCapacity(nodes: Node[], selectors: Record<string, string>, resources: Resources,targetNodeId?:string): Capacity {
  const candidates = nodes.filter((node) => node.status === "ONLINE" && labelsMatch(node, selectors)&&(!targetNodeId||node.id===targetNodeId));
  const cpuNodes = candidates.filter((node) => freeMemory(node) >= resources.memory_bytes && gpuFits(node,resources));
  const memoryNodes = candidates.filter((node) => freeCpu(node) >= resources.cpu_millis && gpuFits(node,resources));
  const gpuNodes = candidates.filter((node) => freeCpu(node) >= resources.cpu_millis && freeMemory(node) >= resources.memory_bytes);
  const feasible = gpuNodes.filter((node) => gpuFits(node,resources));
  const vramCandidates = gpuNodes.flatMap((node) => {
    const values = freeGPUs(node).map((gpu) => gpu.vram_bytes).sort((a, b) => b - a);
    return resources.gpu.count > 0 && values.length >= resources.gpu.count ? [values[resources.gpu.count - 1]] : [];
  });
  const reference = [...feasible].sort((a, b) => (freeMemory(a) - resources.memory_bytes) - (freeMemory(b) - resources.memory_bytes) || a.id.localeCompare(b.id))[0] ?? candidates[0];
  return {
    nodes: candidates,
    feasible,
    cpuMax: Math.max(0, ...cpuNodes.map(freeCpu)),
    memoryMax: Math.max(0, ...memoryNodes.map(freeMemory)),
    gpuMax: Math.max(0, ...gpuNodes.map((node) => freeGPUs(node, resources.gpu.min_vram_bytes).length)),
    vramMax: Math.max(0, ...vramCandidates),
    reference,
  };
}
