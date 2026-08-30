import { describe, expect, it, vi } from "vitest";
import { signOut } from "./App";

describe("signOut", () => {
  it("clears client state and reloads after signing out", async () => {
    const logout = vi.fn().mockResolvedValue(undefined), clear = vi.fn(), reload = vi.fn();
    await signOut(logout, clear, reload);
    expect(logout).toHaveBeenCalledOnce();
    expect(clear).toHaveBeenCalledOnce();
    expect(reload).toHaveBeenCalledOnce();
  });

  it("still reloads when the logout request fails", async () => {
    const clear = vi.fn(), reload = vi.fn();
    await expect(signOut(vi.fn().mockRejectedValue(new Error("offline")), clear, reload)).rejects.toThrow("offline");
    expect(clear).toHaveBeenCalledOnce();
    expect(reload).toHaveBeenCalledOnce();
  });
});
