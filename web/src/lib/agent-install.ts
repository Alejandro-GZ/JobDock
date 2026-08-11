function shellQuote(value: string): string {
  return `'${value.replaceAll("'", `'"'"'`)}'`;
}

export function agentInstallCommand(serverURL: string, token: string, gpu: boolean): string {
  const insecure = serverURL.startsWith("http://") ? " --allow-insecure-http" : "";
  const gpuFlag = gpu ? " --gpu" : "";
  return `curl -fsSL ${shellQuote(`${serverURL}/install-agent.sh`)} | sudo sh -s -- --server ${shellQuote(serverURL)} --token ${shellQuote(token)}${insecure}${gpuFlag}`;
}
