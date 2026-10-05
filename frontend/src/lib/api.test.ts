import { describe, expect, it } from "vitest";
import { apiURL } from "./api";

describe("apiURL", () => {
  it("joins an API path to the configured backend origin", () => {
    expect(apiURL("/v1/health")).toBe("http://localhost:8080/v1/health");
  });
});
