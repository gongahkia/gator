import { describe, expect, it } from "vitest";
import { extractHotCrossBunsDeepLinksFromArgv } from "./deepLinkLaunchArgs";

describe("extractHotCrossBunsDeepLinksFromArgv", () => {
  it("ignores executable paths, app paths, and Chromium flags", () => {
    expect(extractHotCrossBunsDeepLinksFromArgv([
      "/Applications/Hot Cross Buns.app/Contents/MacOS/Hot Cross Buns",
      "/Users/alice/hotcrossbuns://task/not-a-link",
      "--original-process-start-time=13213718723637733",
      "--foo=hotcrossbuns://task/task-1"
    ])).toEqual([]);
  });

  it("extracts validated protocol args regardless of argv position", () => {
    expect(extractHotCrossBunsDeepLinksFromArgv([
      "--original-process-start-time=13213718723637733",
      "hotcrossbuns://today",
      "--disable-gpu",
      "hotcrossbuns://task/task-1"
    ])).toEqual([
      "hotcrossbuns://today",
      "hotcrossbuns://task/task-1"
    ]);
  });

  it("rejects malformed and schema-invalid links", () => {
    expect(extractHotCrossBunsDeepLinksFromArgv([
      "hotcrossbuns://task/%E0%A4%A",
      `hotcrossbuns://task/${"a".repeat(257)}`,
      `hotcrossbuns://search?q=${"a".repeat(201)}`,
      "https://example.com/task/task-1"
    ])).toEqual([]);
  });

  it("deduplicates repeated links without changing order", () => {
    expect(extractHotCrossBunsDeepLinksFromArgv([
      "hotcrossbuns://task/task-1",
      "hotcrossbuns://task/task-1",
      "hotcrossbuns://settings"
    ])).toEqual([
      "hotcrossbuns://task/task-1",
      "hotcrossbuns://settings"
    ]);
  });
});
