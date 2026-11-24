export interface SeedTrack {
  title: string;
  artist: string;
  album: string;
  label: string;
  year: string;
  country: string;
  catalogRef: string;
  moods: string[];
  genres: string[];
}

export interface SearchResult {
  rank: string;
  title: string;
  similarity: string;
}
