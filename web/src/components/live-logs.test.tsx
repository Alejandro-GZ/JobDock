// @vitest-environment jsdom

import {act,cleanup,render,screen} from "@testing-library/react";
import {afterEach,describe,expect,it,vi} from "vitest";
import {LiveLogs} from "./live-logs";

class FakeEventSource{
  static instances:FakeEventSource[]=[];onopen:(()=>void)|null=null;onerror:(()=>void)|null=null;listener?:EventListener;
  constructor(public url:string){FakeEventSource.instances.push(this)}
  addEventListener(type:string,listener:EventListener){if(type==="log")this.listener=listener}
  close=vi.fn();
  emit(text:string,stream:"stdout"|"stderr"="stderr"){this.listener?.(new MessageEvent("log",{data:JSON.stringify({stream,start_offset:0,next_offset:text.length,data:btoa(text)})}))}
}

describe("LiveLogs",()=>{
  afterEach(()=>{cleanup();vi.unstubAllGlobals();FakeEventSource.instances=[]});
  it("can show only stderr and renders its output with the configured stream color",()=>{
    vi.stubGlobal("EventSource",FakeEventSource);render(<LiveLogs jobId="job" attemptId="attempt" streams={["stderr"]} embedded/>);
    expect(screen.queryByText("stdout")).toBeNull();act(()=>FakeEventSource.instances[0].emit("failure"));expect(screen.getByText("failure").getAttribute("data-log-stream")).toBe("stderr");expect(FakeEventSource.instances[0].url).toContain("/logs/stderr/tail");
  });
  it("merges stdout and stderr into one console while preserving stderr color",()=>{
    vi.stubGlobal("EventSource",FakeEventSource);render(<LiveLogs jobId="job" attemptId="attempt" streams={["stdout","stderr"]} embedded/>);expect(FakeEventSource.instances[0].url).toContain("/logs/combined/tail");act(()=>{FakeEventSource.instances[0].onopen?.();FakeEventSource.instances[0].emit("ready\n","stdout");FakeEventSource.instances[0].emit("warning\n","stderr")});expect(screen.getByText("ready").getAttribute("data-log-stream")).toBe("stdout");expect(screen.getByText("warning").getAttribute("data-log-stream")).toBe("stderr");expect(screen.getAllByText("Logs").length).toBeGreaterThan(0);
  });
  it("overlays a compact accessible toolbar without opening another stream",()=>{
    vi.stubGlobal("EventSource",FakeEventSource);render(<LiveLogs jobId="job" attemptId="attempt" streams={["stdout","stderr"]} embedded actions={<button type="button">Configure</button>}/>);
    const toolbar=document.querySelector<HTMLElement>("[data-log-toolbar]"),consoleElement=document.querySelector<HTMLElement>("[data-log-console]");
    expect(toolbar?.className).toContain("absolute");expect(toolbar?.className).toContain("top-0");expect(toolbar?.className).toContain("inset-x-0");expect(consoleElement?.className).toContain("relative");
    expect(screen.getByRole("button",{name:"Configure"})).toBeTruthy();expect(screen.getByText("Connecting").getAttribute("tabindex")).toBe("0");expect(FakeEventSource.instances).toHaveLength(1);
    expect(screen.getByLabelText("Log output").className).toContain("pt-8");
  });
  it("derives text and background colors from log source appearance",()=>{
    vi.stubGlobal("EventSource",FakeEventSource);const {container}=render(<LiveLogs jobId="job" attemptId="attempt" streams={["stdout","stderr"]} embedded appearance={{schema_version:1,background_color:"#112233",series:{"log:stdout":{color:"#abcdef"},"log:stderr":{color:"#fedcba"}}}}/>);const root=container.querySelector<HTMLElement>(".jobdock-log-appearance")!;expect(root.style.getPropertyValue("--log-background")).toBe("#112233");expect(root.style.getPropertyValue("--stdout-color")).toBe("#abcdef");expect(root.style.getPropertyValue("--stderr-color")).toBe("#fedcba");
  });
});
