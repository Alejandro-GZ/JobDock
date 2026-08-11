import { describe, expect, it } from "vitest";
import { agentInstallCommand } from "./agent-install";

describe("agent install command", () => {
  it("creates a pinned HTTPS CPU command", () => {
    const command = agentInstallCommand("https://dock.example.com", "one-use-token", false);
    expect(command).toContain("'https://dock.example.com/install-agent.sh'");
    expect(command).toContain("--server 'https://dock.example.com'");
    expect(command).not.toContain("--gpu");
    expect(command).not.toContain("--allow-insecure-http");
  });

  it("opts into local HTTP and GPU discovery explicitly", () => {
    const command = agentInstallCommand("http://localhost:8080", "token", true);
    expect(command).toContain("--allow-insecure-http --gpu");
  });
});
