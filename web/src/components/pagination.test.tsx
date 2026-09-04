import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import { I18nProvider } from "@/lib/i18n";

import { Pagination, pageItems } from "./pagination";

describe("pageItems", () => {
  it("lists every page when there are at most seven", () => {
    expect(pageItems(1, 1)).toEqual([1]);
    expect(pageItems(3, 7)).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });

  it("keeps a fixed 7-slot layout with a trailing gap near the start", () => {
    expect(pageItems(1, 50)).toEqual([1, 2, 3, 4, 5, 6, "next-gap", 50]);
    expect(pageItems(4, 50)).toEqual([1, 2, 3, 4, 5, 6, "next-gap", 50]);
  });

  it("surrounds the current page with two neighbours and both gaps in the middle", () => {
    expect(pageItems(25, 50)).toEqual([1, "prev-gap", 23, 24, 25, 26, 27, "next-gap", 50]);
  });

  it("keeps a leading gap near the end", () => {
    expect(pageItems(47, 50)).toEqual([1, "prev-gap", 45, 46, 47, 48, 49, 50]);
    expect(pageItems(50, 50)).toEqual([1, "prev-gap", 45, 46, 47, 48, 49, 50]);
  });
});

describe("Pagination", () => {
  const setup = (page: number, total = 500, pageSize = 10) => {
    const onChange = vi.fn();
    render(
      <I18nProvider>
        <Pagination page={page} pageSize={pageSize} total={total} onChange={onChange} />
      </I18nProvider>,
    );
    return onChange;
  };

  it("jumps by five on an ellipsis and clamps to bounds", () => {
    const onChange = setup(25);
    const [prevGap, nextGap] = screen.getAllByTitle(/5/);
    fireEvent.click(prevGap!);
    expect(onChange).toHaveBeenLastCalledWith(20, 10);
    fireEvent.click(nextGap!);
    expect(onChange).toHaveBeenLastCalledWith(30, 10);
  });

  it("commits the jump input on Enter, clamped to the last page, and clears it", () => {
    const onChange = setup(1);
    const input = screen.getByRole("spinbutton") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "999" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith(50, 10);
    expect(input.value).toBe("");
  });

  it("does not emit when the target equals the current page", () => {
    const onChange = setup(1);
    fireEvent.click(screen.getByRole("button", { name: "1" }));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "1" })).toHaveAttribute("aria-current", "page");
  });
});
