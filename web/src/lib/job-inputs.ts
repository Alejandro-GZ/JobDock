import type { JobSpec } from "@/types";

export function inputPath(file: File) {
  return (file.webkitRelativePath || file.name).replaceAll("\\", "/");
}

export function jobFormData(spec: JobSpec, inputs: File[]) {
  const body = new FormData();
  body.append("spec", JSON.stringify(spec));
  for (const file of inputs) body.append(`input:${inputPath(file)}`, file, file.name);
  return body;
}
