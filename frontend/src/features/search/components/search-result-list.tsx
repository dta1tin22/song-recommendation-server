import { ListFilter } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { SearchResult } from "../types/search.types";
import { SearchResultItem } from "./search-result-item";

interface SearchResultListProps {
  results: SearchResult[];
}

export function SearchResultList({ results }: SearchResultListProps) {
  return (
    <section className="flex h-[700px] w-full flex-col overflow-hidden rounded-lg border border-border bg-background shadow-sm lg:w-[58%]">
      <header className="flex h-12 shrink-0 items-center justify-between border-b border-border bg-muted/40 px-4">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold text-foreground">Similar Tracks</span>
          <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
            48
          </span>
        </div>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <ListFilter aria-hidden="true" className="size-3.5" />
          <span>Sort: Similarity</span>
        </div>
      </header>

      <div className="flex-1 divide-y divide-border overflow-y-auto">
        {results.map((result, index) => (
          <SearchResultItem key={result.rank} result={result} active={index === 0} />
        ))}
      </div>

      <footer className="flex h-8 shrink-0 items-center justify-between border-t border-border bg-muted/40 px-4 text-[11px] text-muted-foreground">
        <span>SHOWING {results.length} OF 48 CANDIDATES</span>
        <Button type="button" variant="link" size="sm" className="h-auto p-0 text-[11px]">
          LOAD REMAINING 36 →
        </Button>
      </footer>
    </section>
  );
}
