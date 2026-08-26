import {describe,expect,it} from "vitest";
import {dashboardPalettes,effectiveColors,paletteByRef,quickColors} from "@/lib/dashboard-palettes";
import {widgetIcons} from "@/lib/widget-icons";
import {widgetCatalog} from "@/lib/dashboard-widgets";

describe("dashboard appearance catalog",()=>{
  it("keeps stable unique palettes and valid quick colors",()=>{
    expect(new Set(dashboardPalettes.map(item=>item.id)).size).toBe(dashboardPalettes.length);
    expect(dashboardPalettes.every(item=>item.colors.length>=5&&item.colors.every(color=>/^#[0-9a-f]{6}$/i.test(color)))).toBe(true);
    expect(quickColors.every(color=>/^#[0-9a-f]{6}$/i.test(color))).toBe(true);
  });
  it("uses non-destructive widget over dashboard precedence",()=>{
    expect(effectiveColors({schema_version:1,palette:{id:"midnight",version:1}},{schema_version:1,palette:{id:"warm",version:1}})).toEqual(paletteByRef({id:"warm",version:1})?.colors);
  });
  it("assigns every widget type its own icon",()=>{
    const icons=widgetCatalog.map(item=>widgetIcons[item.type]);
    expect(icons.every(Boolean)).toBe(true);
    expect(new Set(icons).size).toBe(widgetCatalog.length);
  });
});
