// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TemplateDummyWidget } from "@/components/template-dummy-widget";

afterEach(cleanup);

describe("TemplateDummyWidget", () => {
  it.each([
    ["roc_curve", "ROC curve sample preview"],
    ["prediction_vs_actual", "Prediction vs Actual sample preview"],
    ["donut_chart", "Donut chart sample preview"],
    ["treemap", "Treemap sample preview"],
    ["data_grid", "Table / Data grid sample preview"],
  ] as const)("renders a type-specific %s fallback", (type, name) => {
    render(<TemplateDummyWidget type={type} />);
    const preview = screen.getByRole("img", { name });
    expect(preview.getAttribute("data-dummy-widget-type")).toBe(type);
  });

  it("does not represent a scatter fallback as a bar plot", () => {
    const { container } = render(<TemplateDummyWidget type="embedding_scatter" />);
    expect(screen.getByRole("img", { name: "Embedding scatter sample preview" })).toBeTruthy();
    expect(container.querySelectorAll("circle").length).toBeGreaterThan(3);
    expect(container.querySelectorAll("rect")).toHaveLength(0);
  });
});
