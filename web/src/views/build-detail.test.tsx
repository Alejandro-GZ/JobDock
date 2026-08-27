// @vitest-environment jsdom

import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {afterEach,beforeEach,describe,expect,it,vi} from "vitest";
import {ManagedImage} from "@/views/build-detail";

const {success,error}=vi.hoisted(()=>({success:vi.fn(),error:vi.fn()}));

vi.mock("sonner",()=>({toast:{success,error}}));

const reference="jobdock://build/3c6083e1-c802-42f3-b565-5de9da74a830@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const digest="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

beforeEach(()=>{
  success.mockReset();
  error.mockReset();
});
afterEach(cleanup);

describe("ManagedImage",()=>{
  it("shows the immutable reference and copies it",async()=>{
    const writeText=vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator,"clipboard",{configurable:true,value:{writeText}});
    render(<ManagedImage artifactAvailable artifactReference={reference} digest={digest}/>);

    expect(screen.getByText(reference).textContent).toBe(reference);
    expect(screen.getByText(digest).textContent).toBe(digest);
    fireEvent.click(screen.getByRole("button",{name:"Copy image reference"}));

    await waitFor(()=>expect(writeText).toHaveBeenCalledWith(reference));
    expect(success).toHaveBeenCalledWith("Image reference copied");
  });

  it("reports clipboard failures",async()=>{
    Object.defineProperty(navigator,"clipboard",{configurable:true,value:{writeText:vi.fn().mockRejectedValue(new Error("denied"))}});
    render(<ManagedImage artifactAvailable artifactReference={reference} digest={digest}/>);
    fireEvent.click(screen.getByRole("button",{name:"Copy image reference"}));
    await waitFor(()=>expect(error).toHaveBeenCalledWith("Could not copy image reference"));
  });

  it("keeps expired artifacts unavailable",()=>{
    render(<ManagedImage artifactAvailable={false} artifactReference={reference} digest={digest}/>);
    expect(screen.getByText("Managed image expired")).toBeTruthy();
    expect(screen.getByText("Create an explicit rebuild before starting new jobs from this build.")).toBeTruthy();
    expect(screen.queryByText(reference)).toBeNull();
    expect(screen.queryByRole("button",{name:"Copy image reference"})).toBeNull();
  });
});
