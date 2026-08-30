// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Secrets } from "@/views/secrets";

const { createSecret, deleteSecret, secrets } = vi.hoisted(() => ({ createSecret: vi.fn(), deleteSecret: vi.fn(), secrets: vi.fn() }));
vi.mock("@/api", () => ({ api: { createSecret, deleteSecret, secrets } }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

beforeEach(() => {
  createSecret.mockReset().mockResolvedValue({ id: "secret", name: "created", kind: "generic", created_at: new Date().toISOString() });
  deleteSecret.mockReset().mockResolvedValue(undefined);
  secrets.mockReset().mockResolvedValue([]);
});
afterEach(cleanup);

function renderSecrets() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><Secrets /></QueryClientProvider>);
}

describe("Secrets", () => {
  it("creates a generic job secret using the public API kind", async () => {
    renderSecrets();
    fireEvent.click(screen.getByRole("button", { name: "New secret" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "dummy-token" } });
    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "not-sensitive" } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(createSecret).toHaveBeenCalledWith("dummy-token", "not-sensitive", "generic"));
  });

  it("serializes registry fields as Docker AuthConfig JSON", async () => {
    renderSecrets();
    fireEvent.click(screen.getByRole("button", { name: "New secret" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "dummy-registry" } });
    fireEvent.change(screen.getByLabelText("Type"), { target: { value: "registry" } });
    fireEvent.change(screen.getByLabelText("Registry server"), { target: { value: "registry.example.com" } });
    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "robot" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "dummy-password" } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(createSecret).toHaveBeenCalledWith("dummy-registry", JSON.stringify({ serveraddress: "registry.example.com", username: "robot", password: "dummy-password" }), "registry"));
  });
});
