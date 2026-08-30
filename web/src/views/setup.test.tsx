// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Setup } from "@/views/setup";

const { setup } = vi.hoisted(() => ({ setup: vi.fn() }));
vi.mock("@/api", () => ({ api: { setup } }));

beforeEach(() => setup.mockReset().mockResolvedValue({ id: "admin", username: "owner", role: "admin" }));
afterEach(cleanup);

describe("first-run setup", () => {
  it("creates the permanent administrator with the one-time token", async () => {
    const complete = vi.fn();
    render(<Setup status={{required:true,enabled:true,suggested_username:"admin"}} onComplete={complete}/>);
    fireEvent.change(screen.getByLabelText("Setup token"),{target:{value:"one-time-token"}});
    fireEvent.change(screen.getByLabelText("Administrator username"),{target:{value:"owner"}});
    fireEvent.change(screen.getByLabelText("Password"),{target:{value:"correct horse battery"}});
    fireEvent.change(screen.getByLabelText("Confirm password"),{target:{value:"correct horse battery"}});
    fireEvent.click(screen.getByRole("button",{name:"Create administrator"}));
    await waitFor(()=>expect(setup).toHaveBeenCalledWith("one-time-token","owner","correct horse battery"));
    expect(complete).toHaveBeenCalledOnce();
  });

  it("rejects mismatched passwords without sending the setup token", () => {
    render(<Setup status={{required:true,enabled:true,suggested_username:"admin"}} onComplete={vi.fn()}/>);
    fireEvent.change(screen.getByLabelText("Setup token"),{target:{value:"one-time-token"}});
    fireEvent.change(screen.getByLabelText("Password"),{target:{value:"correct horse battery"}});
    fireEvent.change(screen.getByLabelText("Confirm password"),{target:{value:"different password"}});
    fireEvent.click(screen.getByRole("button",{name:"Create administrator"}));
    expect(screen.getByRole("alert").textContent).toBe("Passwords do not match.");
    expect(setup).not.toHaveBeenCalled();
  });

  it("shows a repair state when the setup credential is missing", () => {
    render(<Setup status={{required:true,enabled:false,suggested_username:"admin"}} onComplete={vi.fn()}/>);
    expect(screen.getByText("Setup credential unavailable")).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Create administrator"})).toBeNull();
  });
});
