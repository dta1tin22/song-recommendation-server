import { ArrowRight, Search } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";

const SEEDS = [
  {
    id: "01",
    label: "Aphex Twin - Alberto Balsalm",
    value: "Aphex Twin - Alberto Balsalm [WARP LP66]",
  },
  {
    id: "02",
    label: "Boards of Canada - Roygbiv",
    value: "Boards of Canada - Roygbiv [WARP LP55]",
  },
  {
    id: "03",
    label: "Burial - Archangel",
    value: "Burial - Archangel [HDB002]",
  },
  {
    id: "04",
    label: "Basic Channel - Phylyps Trak",
    value: "Basic Channel - Phylyps Trak [BC-02]",
  },
] as const;

export function MainSearch() {
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        inputRef.current?.focus();
        inputRef.current?.select();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  const applySeed = (value: string) => {
    setQuery(value);
    inputRef.current?.focus();
  };

  return (
    <div className="w-full">
      <form
        className="flex h-[52px] w-full items-center gap-2 rounded-xl border border-border bg-background px-4 shadow-sm transition-all focus-within:ring-2 focus-within:ring-ring"
        onSubmit={(event) => event.preventDefault()}
      >
        <Search aria-hidden="true" className="size-5 shrink-0 text-muted-foreground" />

        <input
          ref={inputRef}
          aria-label="Search artist, track, or URL"
          autoComplete="off"
          className="h-full min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground"
          placeholder="Search artist, track, or album..."
          spellCheck={false}
          type="text"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />

        <span className="hidden items-center rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground sm:inline-flex">
          ⌘K
        </span>

        <Button type="submit" className="h-[38px] shrink-0 gap-2">
          Search
          <ArrowRight aria-hidden="true" className="size-4" />
        </Button>
      </form>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        {SEEDS.map((seed) => (
          <Button
            key={seed.id}
            type="button"
            variant="ghost"
            size="sm"
            className="h-auto gap-1.5 px-2 py-1 text-xs"
            onClick={() => applySeed(seed.value)}
          >
            <span className="font-mono text-muted-foreground">#{seed.id}</span>
            {seed.label}
          </Button>
        ))}
      </div>
    </div>
  );
}
