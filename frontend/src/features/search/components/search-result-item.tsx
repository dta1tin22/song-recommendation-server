import { Disc3 } from "lucide-react";

import { cn } from "@/lib/utils";
import type { SearchResult } from "../types/search.types";

interface SearchResultItemProps {
  result: SearchResult;
  active?: boolean;
}

export function SearchResultItem({ result, active = false }: SearchResultItemProps) {
  return (
    <div
      className={cn(
        "flex h-[52px] items-center gap-3 border-l-[3px] px-3.5 transition-colors",
        active
          ? "border-l-primary bg-primary/10"
          : "border-l-transparent hover:bg-muted",
      )}
    >
      <span
        className={cn(
          "w-6 shrink-0 font-mono text-xs",
          active ? "font-semibold text-primary" : "text-muted-foreground",
        )}
      >
        {result.rank}
      </span>
      <div className="flex size-8 shrink-0 items-center justify-center rounded border border-border bg-muted">
        <Disc3 aria-hidden="true" className="size-4 text-muted-foreground" />
      </div>
      <p className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
        {result.title}
      </p>
      <span className="shrink-0 pl-3 font-mono text-xs font-semibold text-primary">
        {result.similarity}
      </span>
    </div>
  );
}
