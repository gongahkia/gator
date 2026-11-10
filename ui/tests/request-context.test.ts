import { expect, test } from "bun:test"
import { requestContext } from "../src/gator/request-context"
import { state } from "./fixture"

test("request scope keeps selected account identities even when their display metadata is unavailable", () => {
  const current = { ...state(), sources: { id: "scope", selected: ["selected-account", "missing-account"], choices: [
    { id: "selected-account", label: "Work mailbox", service: "gmail", status: "Not checked", access: "read" },
    { id: "other-account", label: "Unselected private mailbox", service: "gmail", status: "Ready", access: "send" },
  ] } }
  const draft = { context: "conversation:first", title: "first", source: "topic:first", text: "A private draft" }
  const text = requestContext(current, draft)
  expect(text).toContain("Work mailbox (gmail; Not checked; read)")
  expect(text).toContain("missing-account (details unavailable; inspect Sources)")
  expect(text).not.toContain("Unselected private mailbox")
  expect(text).not.toContain("Connected accounts selected: None")
  expect(requestContext({ ...state(), work_details: { mode: "inspect", attachments: [], connector_ids: ["legacy-selected"] } }, draft)).toContain("legacy-selected (details unavailable")
  expect(requestContext(undefined, draft)).toContain("Unavailable until the host loads this selection")
})
