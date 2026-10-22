import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { AuctionForm } from "@/components/AuctionForm";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));

function png(name: string) {
  return new File([new Uint8Array([137, 80, 78, 71])], name, { type: "image/png" });
}

// Simulates a real browser: the input's FileList is live, so clearing the
// input's value afterwards empties the same FileList object.
function pickFiles(input: HTMLInputElement, files: File[]) {
  const list = [...files];
  const fileList = {
    get length() { return list.length; },
    item: (i: number) => list[i] ?? null,
    [Symbol.iterator]: function* () { yield* list; },
  } as unknown as FileList;
  Object.defineProperty(input, "files", { configurable: true, get: () => fileList });
  Object.defineProperty(input, "value", { configurable: true, get: () => (list.length ? "fakepath-x" : ""), set: () => { list.length = 0; } });
  fireEvent.change(input);
}

describe("AuctionForm image picker", () => {
  let n = 0;
  beforeEach(() => {
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: () => `blob:preview-${n++}`, revokeObjectURL: () => {} }));
  });
  afterEach(() => vi.unstubAllGlobals());

  it("shows a preview for every picked image", () => {
    const { container } = render(<AuctionForm />);
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    pickFiles(input, [png("a.png"), png("b.png")]);
    expect(screen.getByText(/（2\/4/)).toBeInTheDocument();
    expect(container.querySelectorAll('img[src^="blob:"]')).toHaveLength(2);
  });

  it("never accepts more than 4 images", () => {
    const { container } = render(<AuctionForm />);
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    pickFiles(input, [png("1.png"), png("2.png"), png("3.png")]);
    const again = container.querySelector('input[type="file"]') as HTMLInputElement;
    pickFiles(again, [png("4.png"), png("5.png")]);
    expect(screen.getByText(/（4\/4/)).toBeInTheDocument();
    expect(container.querySelector('input[type="file"]')).toBeNull(); // picker hidden when full
  });
});
