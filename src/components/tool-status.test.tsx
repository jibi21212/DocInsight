import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ToolStatus } from "./tool-status";

describe("ToolStatus", () => {
  it("renders nothing when tools array is empty", () => {
    const { container } = render(<ToolStatus tools={[]} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders one row per tool with the supplied label", () => {
    render(
      <ToolStatus
        tools={[
          { name: "search_documents", label: "Searching for \"taxes\"", done: false },
          { name: "summarize_document", label: "Summarizing report.pdf", done: true },
        ]}
      />,
    );
    expect(screen.getByText('Searching for "taxes"')).toBeInTheDocument();
    expect(screen.getByText("Summarizing report.pdf")).toBeInTheDocument();
  });

  it("applies the done state styling for finished tools", () => {
    const { container } = render(
      <ToolStatus
        tools={[
          { name: "list_documents", label: "Listing documents", done: true },
        ]}
      />,
    );
    // Done rows use the emerald palette class
    const row = container.querySelector(".bg-emerald-50, .bg-emerald-900\\/20");
    expect(row).not.toBeNull();
  });

  it("applies the in-progress styling for unfinished tools", () => {
    const { container } = render(
      <ToolStatus
        tools={[
          { name: "get_document", label: "Reading doc", done: false },
        ]}
      />,
    );
    const row = container.querySelector(".bg-blue-50, .bg-blue-900\\/20");
    expect(row).not.toBeNull();
  });
});
