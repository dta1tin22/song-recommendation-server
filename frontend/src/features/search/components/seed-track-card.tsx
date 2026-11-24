import { ArrowUpRight, Waypoints } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { SeedTrack } from "../types/search.types";

interface SeedTrackCardProps {
  track: SeedTrack;
}

export function SeedTrackCard({ track }: SeedTrackCardProps) {
  const metadata = [
    { label: "Album", value: track.album },
    { label: "Label", value: track.label },
    { label: "Year", value: track.year },
    { label: "Country", value: track.country },
    { label: "Catalog Ref", value: track.catalogRef },
  ];

  return (
    <section className="flex w-full flex-col rounded-lg border border-border bg-background p-6 shadow-sm lg:w-[42%] lg:shrink-0">
      <div className="mb-5 flex items-center justify-between">
        <span className="rounded border border-primary/20 bg-primary/10 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wide text-primary">
          Seed Track
        </span>
        <a
          className="flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-primary"
          href="#"
        >
          Discogs
          <ArrowUpRight aria-hidden="true" className="size-3.5" />
        </a>
      </div>

      <div className="mb-6 flex justify-center">
        <div className="relative flex size-[200px] items-center justify-center overflow-hidden rounded-md border border-border bg-muted shadow-sm">
          <Waypoints aria-hidden="true" className="size-16 text-muted-foreground/50" />
          <span className="absolute bottom-2 left-2 rounded bg-black/70 px-1.5 py-0.5 text-[10px] text-white">
            33 RPM
          </span>
        </div>
      </div>

      <div className="mb-5 text-center lg:text-left">
        <h1 className="mb-1 text-[22px] font-bold leading-tight tracking-tight text-foreground">
          {track.title}
        </h1>
        <p className="text-[15px] font-medium text-muted-foreground">{track.artist}</p>
      </div>

      <dl className="mb-6 flex flex-col text-sm">
        {metadata.map(({ label, value }) => (
          <div
            key={label}
            className="flex items-center justify-between border-b border-border py-2.5"
          >
            <dt className="font-normal text-muted-foreground">{label}</dt>
            <dd className="max-w-[200px] truncate text-right font-medium text-foreground">
              {value}
            </dd>
          </div>
        ))}
      </dl>

      <div className="flex flex-col">
        <div className="mb-4">
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            Mood
          </p>
          <div className="flex flex-wrap gap-1.5">
            {track.moods.map((mood) => (
              <span
                key={mood}
                className="rounded-full bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground"
              >
                {mood}
              </span>
            ))}
          </div>
        </div>
        <div>
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            Genre
          </p>
          <div className="flex flex-wrap gap-1.5">
            {track.genres.map((genre) => (
              <span
                key={genre}
                className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary"
              >
                {genre}
              </span>
            ))}
          </div>
        </div>
      </div>

      <Button type="button" variant="outline" className="mt-6 w-full gap-2">
        <Waypoints aria-hidden="true" className="size-4" />
        Use as Seed Track
      </Button>
    </section>
  );
}
