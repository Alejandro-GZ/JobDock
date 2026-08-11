export const MAX_VISIBLE_LOG_CHARS = 1_000_000;

export type VisibleLog = { text: string; truncated: boolean };

export function appendVisibleLog(current: VisibleLog, addition: string, limit = MAX_VISIBLE_LOG_CHARS): VisibleLog {
  const combined = current.text + addition;
  if (combined.length <= limit) return { text: combined, truncated: current.truncated };
  return { text: combined.slice(combined.length - limit), truncated: true };
}

export function decodeBase64Bytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
  return bytes;
}
