// @vitest-environment jsdom

import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {MemoryRouter} from "react-router-dom";
import {describe,expect,it,vi} from "vitest";
import {NewJob} from "./new-job";

vi.mock("@/api",()=>({api:{nodes:async()=>[],secrets:async()=>[]}}));

describe("NewJob execution source",()=>{
  it("offers Auto, Dockerfile, and OCI with progressive configuration",async()=>{
    const user=userEvent.setup(),client=new QueryClient({defaultOptions:{queries:{retry:false}}});
    render(<QueryClientProvider client={client}><MemoryRouter><NewJob/></MemoryRouter></QueryClientProvider>);
    const automatic=screen.getByRole("button",{name:/Project \(Auto\)/});
    expect(automatic.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("Recommended")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:/Dockerfile/}));
    expect(screen.getByLabelText("Build context")).toBeTruthy();
    expect(screen.getByLabelText("Dockerfile path")).toBeTruthy();
    await user.click(screen.getByRole("button",{name:/OCI image/}));
    expect(screen.getByLabelText("OCI image")).toBeTruthy();
    expect(screen.queryByLabelText("Build context")).toBeNull();
  });
});
