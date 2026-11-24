import { MainSearch } from "../components/main-search";

export function HomePage() {
  return (
    <div className="mx-auto flex min-h-[calc(100vh-14rem)] w-full max-w-[800px] flex-col justify-center py-8">
      <h1 className="mb-2 text-center text-2xl font-semibold tracking-tight text-foreground">
        Archival Similarity Engine
      </h1>
      <p className="mx-auto mb-8 max-w-[460px] text-center text-sm text-muted-foreground">
        Search artist, track, album to 
        analyze acoustic and archival similarity.
      </p>
      <MainSearch />
    </div>
  );
}
