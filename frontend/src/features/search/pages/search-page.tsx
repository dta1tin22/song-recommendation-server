import { useSearchParams } from "react-router";

import { SearchResultList } from "../components/search-result-list";
import { SeedTrackCard } from "../components/seed-track-card";
import type { SeedTrack, SearchResult } from "../types/search.types";

const SEED_TRACK: SeedTrack = {
  title: "Alberto Balsalm",
  artist: "Aphex Twin",
  album: "...I Care Because You Do",
  label: "Warp Records",
  year: "1995",
  country: "UK",
  catalogRef: "WARP LP30",
  moods: ["Melancholic", "Hypnotic", "Warm", "Introspective"],
  genres: ["IDM", "Ambient Techno", "Downtempo"],
};

const RESULTS: SearchResult[] = [
  { rank: "01", title: "Boards of Canada — Roygbiv", similarity: "97.4%" },
  { rank: "02", title: "Burial — Archangel", similarity: "95.8%" },
  { rank: "03", title: "Basic Channel — Phylyps Trak", similarity: "94.2%" },
  { rank: "04", title: "Orbital — Halcyon and On and On", similarity: "93.7%" },
  { rank: "05", title: "Autechre — Vletrmx21", similarity: "92.5%" },
  { rank: "06", title: "Massive Attack — Teardrop", similarity: "91.8%" },
  { rank: "07", title: "Four Tet — Rounds", similarity: "90.4%" },
  { rank: "08", title: "Global Communication — 14 31", similarity: "89.6%" },
  { rank: "09", title: "Biosphere — Poa Alpina", similarity: "88.9%" },
  { rank: "10", title: "Portishead — Roads", similarity: "87.5%" },
  { rank: "11", title: "The Future Sound of London — Papua New Guinea", similarity: "86.8%" },
  { rank: "12", title: "Ulrich Schnauss — A Strangely Isolated Place", similarity: "85.9%" },
];

export function SearchPage() {
  const [searchParams] = useSearchParams();
  const query = searchParams.get("q");

  return (
    <div className="mx-auto w-full max-w-[1120px] p-6">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-y-2 border-b border-border pb-4 font-mono text-xs text-muted-foreground">
        <div className="flex items-center gap-2">
          <span className="font-semibold text-foreground">QUERY</span>
          <span className="text-border">/</span>
          <span className="max-w-[260px] truncate">{query ?? "ARCHIVE CATALOG #WAP059"}</span>
          <span className="text-border">/</span>
          <span className="font-medium text-primary">COSINE DISTANCE RANKED</span>
        </div>
        <div className="flex items-center gap-3">
          <span>
            MATCH POOL: <strong className="font-semibold text-foreground">14,289 TRACKS</strong>
          </span>
          <span className="text-border">|</span>
          <span className="rounded bg-emerald-50 px-2 py-0.5 text-emerald-700">
            CONFIDENCE: OPTIMAL
          </span>
        </div>
      </div>

      <div className="flex flex-col items-start gap-6 lg:flex-row">
        <SeedTrackCard track={SEED_TRACK} />
        <SearchResultList results={RESULTS} />
      </div>

      <div className="mt-8 flex flex-wrap items-center justify-between gap-4 border-t border-border pt-4 font-mono text-[11px] text-muted-foreground">
        <div className="flex flex-wrap items-center gap-4">
          <span>
            ARCHIVAL REVISION:{" "}
            <strong className="font-semibold text-foreground">2024.11-RELEASE</strong>
          </span>
          <span>
            INDEX TYPE:{" "}
            <strong className="font-semibold text-foreground">
              ACOUSTIC SPECTRAL HARMONICS
            </strong>
          </span>
        </div>
        <span>
          KEY SIGNATURE: <strong className="font-semibold text-foreground">C MINOR</strong> •
          TEMPO: <strong className="font-semibold text-foreground">94.2 BPM</strong>
        </span>
      </div>
    </div>
  );
}
