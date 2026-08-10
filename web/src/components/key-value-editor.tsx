import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export type Pair = { id: string; key: string; value: string };
export type PairMode = "environment" | "labels" | "selector";
export const newPair = (): Pair => ({ id: crypto.randomUUID(), key: "", value: "" });
export function pairsToRecord(rows: Pair[]) { return Object.fromEntries(rows.filter((row) => row.key.trim()).map((row) => [row.key.trim(), row.value])); }
export function validatePairs(rows: Pair[], mode: PairMode) {
  const errors: Record<string, string> = {}; const seen = new Set<string>();
  for (const row of rows) {
    const key = row.key.trim(); if (!key && row.value) errors[row.id] = "Key is required";
    if (key && seen.has(key)) errors[row.id] = "Duplicate key"; seen.add(key);
    if (mode === "environment" && key && !/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) errors[row.id] = "Invalid environment variable name";
    if (mode === "environment" && key.startsWith("JOBDOCK_")) errors[row.id] = "JOBDOCK_* is reserved";
  }
  return errors;
}

export function KeyValueEditor({ rows, onChange, mode, keySuggestions = [], valueSuggestions = [] }: { rows: Pair[]; onChange: (rows: Pair[]) => void; mode: PairMode; keySuggestions?: string[]; valueSuggestions?: string[] }) {
  const errors = validatePairs(rows, mode);
  const update = (id: string, field: "key" | "value", value: string) => onChange(rows.map((row) => row.id === id ? { ...row, [field]: value } : row));
  const paste = (event: React.ClipboardEvent<HTMLInputElement>, id: string) => {
    const text = event.clipboardData.getData("text"); if (!text.includes("\n")) return;
    const parsed = text.split(/\r?\n/).filter(Boolean).map((line) => { const index = line.indexOf("="); return { id: crypto.randomUUID(), key: index < 0 ? line.trim() : line.slice(0, index).trim(), value: index < 0 ? "" : line.slice(index + 1) }; });
    event.preventDefault(); onChange([...rows.filter((row) => row.id !== id || row.key || row.value), ...parsed]);
  };
  return <div className="space-y-2">
    <datalist id={`${mode}-keys`}>{keySuggestions.map((value) => <option key={value} value={value}/>)}</datalist>
    <datalist id={`${mode}-values`}>{valueSuggestions.map((value) => <option key={value} value={value}/>)}</datalist>
    {rows.map((row) => <div key={row.id} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_32px] gap-2">
      <div><Input aria-label={`${mode} key`} list={`${mode}-keys`} placeholder="Key" value={row.key} onPaste={(event) => paste(event, row.id)} onChange={(event) => update(row.id, "key", event.target.value)}/>{errors[row.id] && <p className="mt-1 text-xs text-destructive">{errors[row.id]}</p>}</div>
      <Input aria-label={`${mode} value`} list={`${mode}-values`} placeholder="Value" value={row.value} onChange={(event) => update(row.id, "value", event.target.value)}/>
      <Button type="button" variant="ghost" size="icon" aria-label="Remove row" onClick={() => onChange(rows.filter((item) => item.id !== row.id))}><Trash2 className="size-4"/></Button>
    </div>)}
    <Button type="button" variant="outline" size="sm" onClick={() => onChange([...rows, newPair()])}><Plus className="size-4"/> Add row</Button>
  </div>;
}
