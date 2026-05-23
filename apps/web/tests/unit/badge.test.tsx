import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Badge } from "@/components/ui";

describe("Badge", () => {
  it("renders the label", () => {
    render(<Badge>Active</Badge>);
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("applies success variant styles", () => {
    render(<Badge variant="success">OK</Badge>);
    const node = screen.getByText("OK");
    expect(node.className).toMatch(/border-srapi-success/);
  });
});
