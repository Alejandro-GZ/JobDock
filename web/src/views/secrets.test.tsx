// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
    fireEvent.click(screen.getByRole("button", { name: "Create secret" }));
    await waitFor(() => expect(createSecret).toHaveBeenCalledWith("dummy-token", "not-sensitive", "generic"));
  });

  it("serializes registry fields as Docker AuthConfig JSON", async () => {
    renderSecrets();
    fireEvent.click(screen.getByRole("button", { name: "New secret" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "dummy-registry" } });
    fireEvent.click(screen.getByRole("button", { name: /Registry credential/ }));
    fireEvent.change(screen.getByLabelText("Registry server"), { target: { value: "registry.example.com" } });
    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "robot" } });
    fireEvent.change(screen.getByLabelText("Password or token"), { target: { value: "dummy-password" } });
    fireEvent.click(screen.getByRole("button", { name: "Create secret" }));
    await waitFor(() => expect(createSecret).toHaveBeenCalledWith("dummy-registry", JSON.stringify({ serveraddress: "registry.example.com", username: "robot", password: "dummy-password" }), "registry"));
  });

  it("validates fields inline and can reveal the write-only value", () => {
    renderSecrets();
    fireEvent.click(screen.getByRole("button", { name: "New secret" }));
    fireEvent.click(screen.getByRole("button", { name: "Create secret" }));
    expect(screen.getAllByRole("alert").map(item => item.textContent)).toEqual(["Name is required.", "Value is required."]);
    const value = screen.getByLabelText("Value") as HTMLInputElement;
    expect(value.type).toBe("password");
    fireEvent.click(screen.getByRole("button", { name: "Show secret value" }));
    expect(value.type).toBe("text");
    expect(createSecret).not.toHaveBeenCalled();
  });

  it("searches and filters the secret inventory", async () => {
    secrets.mockResolvedValue([
      { id: "generic", name: "model-token", kind: "generic", created_at: "2026-08-30T10:00:00Z" },
      { id: "registry", name: "ghcr", kind: "registry", created_at: "2026-08-30T10:00:00Z" },
    ]);
    renderSecrets();
    expect(await screen.findByText("model-token")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Registry credentials/ }));
    expect(screen.queryByText("model-token")).toBeNull();
    expect(screen.getByText("ghcr")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Search secrets"), { target: { value: "missing" } });
    expect(screen.getByText("No matching secrets")).toBeTruthy();
  });

  it("requires confirmation before deleting a secret", async () => {
    const user = userEvent.setup();
    secrets.mockResolvedValue([{ id: "generic", name: "model-token", kind: "generic", created_at: "2026-08-30T10:00:00Z" }]);
    renderSecrets();
    await screen.findByText("model-token");
    await user.click(screen.getByRole("button", { name: "Actions for model-token" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    expect(screen.getByText("Delete model-token?")).toBeTruthy();
    expect(deleteSecret).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Delete secret" }));
    await waitFor(() => expect(deleteSecret).toHaveBeenCalledWith("generic", expect.anything()));
  });
});
