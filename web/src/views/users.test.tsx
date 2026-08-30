// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Users } from "@/views/users";

const { users, createUser } = vi.hoisted(() => ({ users: vi.fn(), createUser: vi.fn() }));
vi.mock("@/api", () => ({ api: { users, createUser } }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

beforeEach(() => {
  users.mockReset().mockResolvedValue([{ id: "user-123456789", username: "operator", role: "member", created_at: "2026-08-01T10:00:00Z", last_seen_at: new Date().toISOString(), jobs_running: 2 }]);
  createUser.mockReset().mockResolvedValue(undefined);
});
afterEach(cleanup);

describe("Users", () => {
  it("shows operational activity and keeps user creation in the table header", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><Users/></QueryClientProvider>);
    expect(await screen.findByText("operator")).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Last seen" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Jobs running" })).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
    const headers = screen.getAllByRole("columnheader");
    expect(within(headers[headers.length - 1]).getByRole("button", { name: "New user" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Users" })).toBeNull();
  });
});
