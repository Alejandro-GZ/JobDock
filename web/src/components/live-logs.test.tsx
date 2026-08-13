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
  it("can show only stderr and renders its output in red",()=>{
    vi.stubGlobal("EventSource",FakeEventSource);render(<LiveLogs jobId="job" attemptId="attempt" streams={["stderr"]} embedded/>);
    expect(screen.queryByText("stdout")).toBeNull();act(()=>FakeEventSource.instances[0].emit("failure"));const output=screen.getByText("failure");expect(output.className).toContain("text-red-300");expect(FakeEventSource.instances[0].url).toContain("/logs/stderr/tail");
  });
  it("merges stdout and stderr into one console while preserving stderr color",()=>{
    vi.stubGlobal("EventSource",FakeEventSource);render(<LiveLogs jobId="job" attemptId="attempt" streams={["stdout","stderr"]} embedded/>);act(()=>{FakeEventSource.instances[0].emit("ready\n","stdout");FakeEventSource.instances[1].emit("warning\n","stderr")});expect(screen.getByText("ready").className).toBe("");expect(screen.getByText("warning").className).toContain("text-red-300");expect(screen.getAllByText("Logs").length).toBeGreaterThan(0);
  });
});
