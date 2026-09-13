import { useEffect, useMemo, useState } from "react";
import type { SearchResultItem } from "@shared/ipc/contracts";
import { parseLocalSearchQuery } from "@shared/search/localSearch";
import { reportRendererTiming } from "../../../hooks/useRenderTiming";
import type { SearchViewModel } from "../coreViewModels";
import { unwrap } from "./result";
import type { SearchHookState } from "./types";

const LOCAL_SEARCH_DEBOUNCE_MS = 12;

export function useLocalSearch(query: string): SearchHookState {
  const parsed = useMemo(() => parseLocalSearchQuery(query), [query]);
  const [state, setState] = useState<SearchHookState>({
    viewModel: idleSearchViewModel(),
    state: "idle",
    parsed
  });

  useEffect(() => {
    const trimmed = query.trim();

    if (!trimmed) {
      setState({
        viewModel: idleSearchViewModel(),
        state: "idle",
        parsed
      });
      return;
    }

    if (parsed.errors.length > 0) {
      setState((current) => ({
        viewModel:
          current.viewModel.state === "results"
            ? current.viewModel
            : emptySearchViewModel("Fix the query syntax to search planner data."),
        state: "invalid",
        parsed,
        errorMessage: parsed.errors[0]?.message ?? "Invalid search query."
      }));
      return;
    }

    if (!window.hcb) {
      setState({
        viewModel: emptySearchViewModel("Search is unavailable while the preload bridge is offline."),
        state: "offline",
        parsed,
        errorMessage: "Preload bridge is unavailable."
      });
      return;
    }

    let cancelled = false;
    const debounce = window.setTimeout(() => {
      const startedAt = performance.now();

      setState((current) => ({
        viewModel: current.viewModel,
        state: current.viewModel.state === "results" ? "stale" : "loading",
        parsed
      }));

      window.hcb?.search
        .query({
          query: trimmed,
          limit: 30
        })
        .then((result) => unwrap(result, "Search failed"))
        .then((response) => {
          if (cancelled) {
            return;
          }

          const viewModel = searchViewModelFromResults(trimmed, response.items);

          setState({
            viewModel,
            state: viewModel.state,
            parsed,
            latencyMs: Math.max(0, Math.round(performance.now() - startedAt))
          });
          reportRendererTiming("search.query", performance.now() - startedAt, {
            resultCount: response.items.length,
            state: viewModel.state
          });
        })
        .catch((error: unknown) => {
          if (cancelled) {
            return;
          }

          setState({
            viewModel: emptySearchViewModel("Local search could not read the cache."),
            state: "error",
            parsed,
            errorMessage: error instanceof Error ? error.message : "Local search failed."
          });
        });
    }, LOCAL_SEARCH_DEBOUNCE_MS);

    return () => {
      cancelled = true;
      window.clearTimeout(debounce);
    };
  }, [query, parsed]);

  return state;
}

export function searchViewModelFromResults(query: string, items: SearchResultItem[]): SearchViewModel {
  if (items.length === 0) {
    return {
      state: "empty",
      summary: "0 results",
      results: []
    };
  }

  return {
    state: "results",
    summary: `${items.length} ${items.length === 1 ? "result" : "results"}`,
    results: items.map((item) => ({
      id: `${item.domain}-${item.id}`,
      targetId: item.id,
      source: item.domain === "calendar" ? "event" : item.domain === "tasks" ? "task" : "note",
      title: item.title,
      detail: searchResultDetail(query, item),
      snoozeUntil: item.domain === "tasks" ? item.snoozeUntil ?? null : undefined,
      deepLinkLabel: `hotcrossbuns://${item.domain}/${item.id}`
    }))
  };
}

function searchResultDetail(query: string, item: SearchResultItem): string {
  const detail = item.snippet ?? `Matched "${query}"`;

  if (item.domain !== "tasks" || !item.snoozeUntil) {
    return detail;
  }

  return `${detail} - ${snoozeSearchLabel(item.snoozeUntil)}`;
}

function snoozeSearchLabel(value: string): string {
  const date = new Date(value);

  return Number.isNaN(date.getTime())
    ? "Snoozed"
    : `Snoozed ${date.toLocaleString([], {
        dateStyle: "medium",
        timeStyle: "short"
      })}`;
}

export function idleSearchViewModel(): SearchViewModel {
  return {
    state: "idle",
    summary: "Planner data",
    results: []
  };
}

export function emptySearchViewModel(summary: string): SearchViewModel {
  return {
    state: "empty",
    summary,
    results: []
  };
}
