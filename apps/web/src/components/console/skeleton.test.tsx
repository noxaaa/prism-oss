import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ActivityIcon } from "lucide-react";
import { DataState, SummaryCard, TableSkeleton } from "./shared";

describe("console skeleton components", () => {
  it("renders DataState loading as a table skeleton by default", () => {
    const markup = renderToStaticMarkup(
      <DataState error="" loading>
        <div>Loaded content</div>
      </DataState>,
    );

    expect(markup).toContain("data-console-table-skeleton=\"true\"");
    expect(markup).toContain("<table");
    expect(markup).not.toContain("Loaded content");
  });

  it("uses custom loadingFallback for table dimensions", () => {
    const markup = renderToStaticMarkup(
      <DataState error="" loading loadingFallback={<TableSkeleton columns={5} rows={2} />}>
        <div>Loaded content</div>
      </DataState>,
    );

    expect(markup.match(/<th[ >]/g)?.length).toBe(5);
    expect(markup.match(/<tr/g)?.length).toBe(3);
  });

  it("keeps summary card structure while replacing only the value", () => {
    const markup = renderToStaticMarkup(
      <SummaryCard icon={<ActivityIcon />} label="Active rules" loading value={42} />,
    );

    expect(markup).toContain("Active rules");
    expect(markup).not.toContain(">42<");
    expect(markup).toContain("data-slot=\"skeleton\"");
  });
});
