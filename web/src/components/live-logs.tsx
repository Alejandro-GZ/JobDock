import { useEffect, useRef, useState } from "react";
import { AlertCircle, Radio } from "lucide-react";
import { api } from "@/api";
import { appendVisibleLog, decodeBase64Bytes, type VisibleLog } from "@/lib/live-logs";

type StreamName = "stdout" | "stderr";
type ConnectionState = "connecting" | "live" | "reconnecting";
type Chunk = { stream: StreamName; start_offset: number; next_offset: number; data: string };

export function LiveLogs({ jobId }: { jobId: string }) {
  return <div className="grid gap-3 xl:grid-cols-2"><LiveLog jobId={jobId} stream="stdout"/><LiveLog jobId={jobId} stream="stderr"/></div>;
}

function LiveLog({ jobId, stream }: { jobId: string; stream: StreamName }) {
  const [log, setLog] = useState<VisibleLog>({ text: "", truncated: false });
  const [connection, setConnection] = useState<ConnectionState>("connecting");
  const cursor = useRef<number | undefined>(undefined);
  const decoder = useRef(new TextDecoder());
  const output = useRef<HTMLPreElement>(null);

  useEffect(() => {
    setLog({ text: "", truncated: false });
    setConnection("connecting");
    cursor.current = undefined;
    decoder.current = new TextDecoder();
    const source = api.openLogStream(jobId, stream);
    source.onopen = () => setConnection("live");
    source.onerror = () => setConnection("reconnecting");
    source.addEventListener("log", (event) => {
      const chunk = JSON.parse((event as MessageEvent<string>).data) as Chunk;
      const bytes = decodeBase64Bytes(chunk.data);
      const knownOffset = cursor.current;
      const duplicateBytes = knownOffset === undefined ? 0 : Math.max(0, knownOffset - chunk.start_offset);
      if (duplicateBytes >= bytes.length) return;
      const text = decoder.current.decode(bytes.subarray(duplicateBytes), { stream: true });
      cursor.current = chunk.next_offset;
      setLog(current => appendVisibleLog(current, text));
      setConnection("live");
    });
    return () => source.close();
  }, [jobId, stream]);

  useEffect(() => { if (output.current) output.current.scrollTop = output.current.scrollHeight; }, [log.text]);
  const live = connection === "live";
  return <div className="overflow-hidden rounded-md border">
    <div className="flex items-center justify-between border-b bg-muted/40 px-3 py-2 font-mono text-xs">
      <span>{stream}</span>
      <span className={live ? "flex items-center gap-1.5 text-emerald-600" : "flex items-center gap-1.5 text-amber-600"} title={live ? "Receiving incremental log updates" : "The stream will reconnect automatically"}>
        {live ? <Radio className="size-3.5"/> : <AlertCircle className="size-3.5"/>}{live ? "Live" : connection === "connecting" ? "Connecting" : "Reconnecting"}
      </span>
    </div>
    {log.truncated && <p className="border-b bg-amber-500/10 px-3 py-1.5 text-xs text-amber-700 dark:text-amber-300">Earlier output is hidden to keep the browser responsive. The complete log remains available in the ZIP.</p>}
    <pre ref={output} className="h-[55vh] overflow-auto bg-zinc-950 p-4 text-xs text-zinc-100">{log.text || "No output yet."}</pre>
  </div>;
}
