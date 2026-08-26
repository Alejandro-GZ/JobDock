// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { ObservabilityDashboard } from "./observability-dashboard";
import { TooltipProvider } from "@/components/ui/tooltip";
const wrap = (node: React.ReactNode) => <TooltipProvider>{node}</TooltipProvider>;
beforeAll(() => Object.defineProperties(HTMLElement.prototype, {
    hasPointerCapture: { configurable: true, value: () => false },
    setPointerCapture: { configurable: true, value: () => undefined },
    scrollIntoView: { configurable: true, value: () => undefined },
}));
describe("ObservabilityDashboard", () => {
    afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
    it("keeps layout controls in edit mode and adds widgets by dropping from the library", async () => {
        const changed = vi.fn();
        render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[]} matrices={[]} markers={[]} initialWidgets={[]} onWidgetsChange={changed}/>));
        expect(await screen.findByText("Drag a widget from the library into this area.")).toBeTruthy();
        for (const heading of ["Trends", "Relationships", "Summaries", "Diagnostics", "Operational"])
            expect(screen.getByRole("heading", { name: heading })).toBeTruthy();
        const palette = screen.getByText("Bar plot").closest("[draggable=true]")!, zone = screen.getByLabelText("Metrics dashboard").querySelector(".relative.min-w-0")!;
        fireEvent.dragStart(palette, { dataTransfer: dataTransfer() });
        fireEvent.dragOver(zone, { dataTransfer: dataTransfer() });
        fireEvent.drop(zone, { dataTransfer: dataTransfer() });
        await waitFor(() => expect(document.querySelector('[data-widget-type="barplot"]')).toBeTruthy());
        expect(screen.getByRole("button", { name: "Remove widget" })).toBeTruthy();
        const handle = screen.getByRole("button", { name: "Drag to resize widget" });
        fireEvent.pointerDown(handle, { clientX: 0, clientY: 0, pointerId: 1 });
        fireEvent.pointerMove(window, { clientX: 120, clientY: 90, pointerId: 1 });
        await waitFor(() => expect(document.querySelector<HTMLElement>('[data-widget-type="barplot"]')?.dataset.size).toBe("7x4"));
        fireEvent.pointerUp(window, { clientX: 120, clientY: 90, pointerId: 1 });
        expect(document.querySelector<HTMLElement>('[data-widget-type="barplot"]')?.dataset.size).toBe("7x4");
    });
    it("renders real values only outside edit mode", async () => {
        const source = { kind: "metric" as const, name: "loss", title: "loss", unit: "ratio", points: [{ timestamp: 1000, value: .5 }] }, saved = [{ id: "loss", type: "lineplot" as const, size: { columns: 2, rows: 1 }, position: { x: 0, y: 0 }, sources: [{ kind: "metric" as const, name: "loss" }], grid_columns: 4 as const }];
        const { rerender } = render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect(await screen.findByRole("img", { name: /Line plot lineplot/ })).toBeTruthy();
        rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect(screen.queryByRole("img", { name: /Line plot lineplot/ })).toBeNull();
        expect(screen.getByText("metric / loss")).toBeTruthy();
    });
    it("preconfigures declared phase sources and activates them when data arrives", async () => {
        const waiting = { kind: "metric" as const, name: "validation_loss", title: "validation_loss", unit: "ratio", points: [], phase: "validation", declared: true, observed: false }, saved = [{ id: "future-loss", type: "lineplot" as const, size: { columns: 6, rows: 3 }, position: { x: 0, y: 0 }, sources: [{ kind: "metric" as const, name: "validation_loss" }], grid_columns: 12 as const }];
        const { rerender } = render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[waiting]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect(await screen.findByText("Waiting for data")).toBeTruthy();
        expect(screen.getByText("Phase: validation")).toBeTruthy();
        rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[{ ...waiting, observed: true, points: [{ timestamp: 1000, value: .4 }] }]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect(await screen.findByRole("img", { name: /Line plot lineplot with 1 points/ })).toBeTruthy();
        expect(screen.queryByText("Waiting for data")).toBeNull();
    });
    it("offers declared matrix sources before observations exist", async () => {
        const user = userEvent.setup(), descriptors = [{ name: "validation_confusion", type: "matrix", phase: "validation", declared: true, observed: false }], saved = [{ id: "matrix", type: "confusion_matrix" as const, size: { columns: 6, rows: 3 }, position: { x: 0, y: 0 }, sources: [{ kind: "matrix" as const, name: "validation_confusion" }], grid_columns: 12 as const }];
        const { rerender } = render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[]} observableSources={descriptors} matrices={[]} markers={[]} initialWidgets={saved}/>));
        await user.click(screen.getByRole("button", { name: "Configure widget data" }));
        expect(screen.getByRole("combobox", { name: "Source" }).textContent).toContain("validation · validation_confusion");
        await user.click(screen.getByRole("button", { name: "Cancel" }));
        rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[]} observableSources={descriptors} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect(await screen.findByText("Waiting for data")).toBeTruthy();
    });
    it("does not query declared table sources until an observation exists", async () => {
        const fetchMock = vi.fn();
        vi.stubGlobal("fetch", fetchMock);
        const descriptors = [{ name: "category_snapshot", type: "table", phase: "evaluation", declared: true, observed: false, tags: ["table:categorical"] }], saved = [{ id: "pie", type: "pie_chart" as const, size: { columns: 6, rows: 4 }, position: { x: 0, y: 0 }, sources: [{ kind: "table" as const, name: "category_snapshot" }], grid_columns: 12 as const }];
        render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[]} observableSources={descriptors} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect(await screen.findByText("Waiting for data")).toBeTruthy();
        expect(screen.getByText("Phase: evaluation")).toBeTruthy();
        expect(fetchMock).not.toHaveBeenCalled();
    });
    it("materializes a template replacement as an ordinary editable dashboard", async () => {
        const source = { kind: "metric" as const, name: "loss", title: "loss", unit: "ratio", points: [] }, saved = [{ id: "logs", type: "logs" as const, size: { columns: 4, rows: 2 }, position: { x: 0, y: 0 }, sources: [{ kind: "log" as const, name: "stdout" }], grid_columns: 4 as const }], replacement = [{ id: "loss", type: "lineplot" as const, size: { columns: 2, rows: 1 }, position: { x: 0, y: 0 }, sources: [{ kind: "metric" as const, name: "loss" }], grid_columns: 4 as const }], changed = vi.fn();
        const { rerender } = render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved} onWidgetsChange={changed}/>));
        expect((await screen.findAllByText("log / stdout")).length).toBeGreaterThan(0);
        rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved} replacement={{ key: "template-1", widgets: replacement }} onWidgetsChange={changed}/>));
        expect(await screen.findByText("metric / loss")).toBeTruthy();
        expect(screen.getByRole("button", { name: "Configure widget data" })).toBeTruthy();
        expect(screen.getByRole("button", { name: "Remove widget" })).toBeTruthy();
        expect(changed).not.toHaveBeenCalled();
    });
    it("previews tile reflow during drag and commits it only on drop", async () => {
        const source = { kind: "metric" as const, name: "loss", title: "loss", unit: "ratio", points: [] }, saved = ["first", "second"].map((id, index) => ({ id, type: "lineplot" as const, size: { columns: 2, rows: 1 }, position: { x: index * 2, y: 0 }, sources: [{ kind: "metric" as const, name: "loss" }], grid_columns: 4 as const }));
        render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        await waitFor(() => expect(document.querySelectorAll("[data-widget-id]")).toHaveLength(2));
        const transfer = dataTransfer(), tiles = () => [...document.querySelectorAll<HTMLElement>("[data-widget-id]")];
        fireEvent.dragStart(tiles()[1].querySelector("section")!, { dataTransfer: transfer });
        fireEvent.dragOver(tiles()[0], { dataTransfer: transfer });
        await waitFor(() => expect(tiles().map(tile => tile.dataset.widgetId)).toEqual(["second", "first"]));
        fireEvent.drop(tiles()[0], { dataTransfer: transfer });
        expect(tiles().map(tile => tile.dataset.widgetId)).toEqual(["second", "first"]);
    });
    it("previews paint targets and creates three-stop gradients from quick colors",async()=>{
        const changed=vi.fn(),source={kind:"metric" as const,name:"temperature",title:"Temperature",unit:"°C",points:[{timestamp:1,value:40}]},saved=[{id:"gauge",type:"gauge" as const,size:{columns:4,rows:3},position:{x:0,y:0},sources:[{kind:"metric" as const,name:"temperature"}],grid_columns:4 as const}];
        render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved} onWidgetsChange={changed}/>));await screen.findByText("metric / temperature");const color=screen.getByRole("button",{name:"Apply #2563eb"}),clear=screen.getByRole("button",{name:"Remove explicit color"}),tile=document.querySelector<HTMLElement>('[data-widget-id="gauge"]')!,transfer=dataTransfer();fireEvent.dragStart(color,{dataTransfer:transfer});fireEvent.dragOver(tile,{dataTransfer:transfer});expect(tile.querySelector('[data-paint-preview="widget"]')).toBeTruthy();fireEvent.drop(tile,{dataTransfer:transfer});await waitFor(()=>expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.gradient).toHaveLength(3));expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.accent_color).toBe("#2563eb");const sourceTarget=tile.querySelector<HTMLElement>("[data-series-target]")!,sourceColor=dataTransfer();fireEvent.dragStart(color,{dataTransfer:sourceColor});fireEvent.dragOver(sourceTarget,{dataTransfer:sourceColor});expect(sourceTarget.dataset.paintPreview).toBe("series");fireEvent.drop(sourceTarget,{dataTransfer:sourceColor});await waitFor(()=>expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.series["metric:temperature"].color).toBe("#2563eb"));const sourceClear=dataTransfer();fireEvent.dragStart(clear,{dataTransfer:sourceClear});fireEvent.dragOver(sourceTarget,{dataTransfer:sourceClear});fireEvent.drop(sourceTarget,{dataTransfer:sourceClear});await waitFor(()=>expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.series["metric:temperature"]).toBeUndefined());const widgetClear=dataTransfer();fireEvent.dragStart(clear,{dataTransfer:widgetClear});fireEvent.dragOver(tile,{dataTransfer:widgetClear});fireEvent.drop(tile,{dataTransfer:widgetClear});await waitFor(()=>expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.gradient).toBeUndefined());expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.accent_color).toBeUndefined();
    });
    it("keeps a custom color until it is explicitly applied or dragged",async()=>{
        const changed=vi.fn(),source={kind:"metric" as const,name:"temperature",title:"Temperature",unit:"°C",points:[{timestamp:1,value:40}]},saved=[{id:"gauge",type:"gauge" as const,size:{columns:4,rows:3},position:{x:0,y:0},sources:[{kind:"metric" as const,name:"temperature"}],grid_columns:4 as const}];
        render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved} onWidgetsChange={changed}/>));await screen.findByText("metric / temperature");const tile=document.querySelector<HTMLElement>('[data-widget-id="gauge"]')!;fireEvent.click(tile.querySelector("section")!);fireEvent.change(screen.getByLabelText("Custom color hex"),{target:{value:"#123456"}});expect(changed).not.toHaveBeenCalled();await userEvent.click(screen.getByRole("button",{name:"Apply custom color"}));await waitFor(()=>expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.accent_color).toBe("#123456"));expect(changed.mock.calls.at(-1)?.[0]?.[0].appearance.gradient).toHaveLength(3);const transfer=dataTransfer();fireEvent.dragStart(screen.getByRole("button",{name:"Drag custom color #123456"}),{dataTransfer:transfer});expect(transfer.getData("text/jobdock-appearance-color")).toBe("#123456");
    });
    it("configures widget data and time range from its editing shell", async () => {
        const user = userEvent.setup(), changed = vi.fn(), sources = [{ kind: "metric" as const, name: "loss", title: "loss", unit: "ratio", points: [{ timestamp: 1, value: .8, step: 1 }] }, { kind: "metric" as const, name: "duration", title: "duration", unit: "seconds", points: [{ timestamp: 1, value: 8, step: 1 }] }], saved = [{ id: "loss", type: "lineplot" as const, size: { columns: 2, rows: 1 }, position: { x: 0, y: 0 }, sources: [{ kind: "metric" as const, name: "loss" }], grid_columns: 4 as const }];
        render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={sources} matrices={[]} markers={[]} initialWidgets={saved} onWidgetsChange={changed}/>));
        await user.click(screen.getByRole("button", { name: "Configure widget data" }));
        expect(screen.getByRole("combobox", { name: "Time range" })).toBeTruthy();
        expect(screen.queryByRole("combobox", { name: "Color scheme" })).toBeNull();
        screen.getByRole("combobox", { name: "Series to add" }).focus();
        await user.keyboard("{Enter}{ArrowDown}{Enter}");
        await user.click(screen.getByRole("button", { name: "Add selected series" }));
        await user.click(screen.getByRole("button", { name: "Apply" }));
        expect(await screen.findByText("metric / loss")).toBeTruthy();
        expect(screen.getByText("metric / duration")).toBeTruthy();
        await user.click(screen.getByRole("button", { name: "Configure widget appearance" }));
        expect(screen.getByRole("combobox", { name: "Color scheme" })).toBeTruthy();
        expect(screen.getByRole("combobox", { name: "Legend" })).toBeTruthy();
        expect(screen.getByRole("combobox", { name: "Line style" })).toBeTruthy();
        await user.type(screen.getByLabelText("Widget title"), "Training signals");
        await user.type(screen.getByLabelText("Widget subtitle"), "Validation split");
        await user.click(screen.getByRole("checkbox", { name: "Show points" }));
        await user.clear(screen.getByLabelText("Y axis label"));
        await user.type(screen.getByLabelText("Y axis label"), "Objective");
        await user.clear(screen.getByLabelText("loss display label"));
        await user.type(screen.getByLabelText("loss display label"), "Loss");
        await user.click(screen.getByRole("button", { name: "Apply" }));
        await waitFor(() => expect(changed.mock.calls.at(-1)?.[0]?.[0]).toMatchObject({ title: "Training signals", appearance: { schema_version: 1, subtitle: "Validation split", show_points: true, y_axis: { label: "Objective" }, series: { "metric:loss": { label: "Loss" } } } }));
    });
    it("renders a legacy gauge and exposes its migrated domain in edit mode", async () => {
        const user = userEvent.setup(), source = { kind: "metric" as const, name: "temperature", title: "Temperature", unit: "°C", points: [{ timestamp: 1, value: 20 }, { timestamp: 2, value: 40 }] }, saved = [{ id: "gauge", type: "gauge" as const, size: { columns: 6, rows: 3 }, position: { x: 0, y: 0 }, sources: [{ kind: "metric" as const, name: "temperature" }], grid_columns: 12 as const, gauge_max_mode: "fixed" as const, gauge_max_value: 100 }];
        const { rerender } = render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect(await screen.findByRole("meter", { name: "Temperature: 40 °C" })).toBeTruthy();
        rerender(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready editMode numericSources={[source]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        await user.click(screen.getByRole("button", { name: "Configure widget data" }));
        expect(screen.getByRole("combobox", { name: "Gauge presentation" }).textContent).toContain("Gauge");
        expect((screen.getByLabelText("Maximum") as HTMLInputElement).value).toBe("100");
    });
    it("shows a single combined Logs console with stream selection beside its title", async () => {
        class EventSourceStub {
            onopen = null;
            onerror = null;
            addEventListener() { }
            close() { }
        }
        vi.stubGlobal("EventSource", EventSourceStub);
        const user = userEvent.setup(), saved = [{ id: "logs", type: "logs" as const, size: { columns: 4, rows: 2 }, position: { x: 0, y: 0 }, sources: [{ kind: "log" as const, name: "stdout" }], grid_columns: 4 as const }];
        render(wrap(<ObservabilityDashboard jobID="job" attemptID="attempt" ready numericSources={[]} matrices={[]} markers={[]} initialWidgets={saved}/>));
        expect((await screen.findAllByText("Logs")).length).toBeGreaterThan(0);
        expect(screen.queryByText("stdout")).toBeNull();
        await user.click(screen.getByRole("button", { name: "Configure widget data" }));
        expect((screen.getByRole("checkbox", { name: "stdout" }) as HTMLInputElement).checked).toBe(true);
        await user.click(screen.getByRole("checkbox", { name: "stderr" }));
        await user.click(screen.getByRole("button", { name: "Apply" }));
    });
});
function dataTransfer() { const values = new Map<string, string>(); return { effectAllowed: "all", dropEffect: "move", files: [], items: [], get types(){return[...values.keys()]}, setData: (key: string, value: string) => values.set(key, value), getData: (key: string) => values.get(key) ?? "", clearData: () => values.clear(), setDragImage: () => undefined }; }
