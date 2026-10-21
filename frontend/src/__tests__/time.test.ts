import { describe, expect, it } from "vitest";
import { formatRemaining, formatTaipei, isoToTaipeiLocalInput, taipeiLocalInputToISO } from "@/lib/time";

describe("time helpers", () => {
  it("formats UTC instants in Asia/Taipei", () => {
    expect(formatTaipei("2026-10-21T16:30:00Z")).toBe("2026/10/22 00:30");
    expect(formatTaipei("not a date")).toBe("");
  });

  it("round-trips datetime-local values as Taipei time", () => {
    expect(taipeiLocalInputToISO("2026-10-22T00:30")).toBe("2026-10-21T16:30:00.000Z");
    expect(isoToTaipeiLocalInput("2026-10-21T16:30:00Z")).toBe("2026-10-22T00:30");
    expect(() => taipeiLocalInputToISO("garbage")).toThrow();
  });

  it("formats remaining time", () => {
    expect(formatRemaining(0)).toBe("已結束");
    expect(formatRemaining(45_000)).toBe("45 秒");
    expect(formatRemaining(125_000)).toBe("2 分 5 秒");
    expect(formatRemaining(2 * 3600_000 + 60_000)).toBe("2 小時 1 分");
    expect(formatRemaining(49 * 3600_000)).toBe("2 天 1 小時");
  });
});
