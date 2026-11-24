import { UserRound } from "lucide-react";
import { Link } from "react-router";

import { PATHS } from "@/app/routes/paths";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const NAV_ITEMS = [
  { id: "search", label: "Search", to: PATHS.search },
  { id: "random-seed", label: "Random Seed", to: "#" },
  { id: "history", label: "History", to: "#" },
  { id: "api", label: "API", to: "#" },
] as const;

type ActiveItem = (typeof NAV_ITEMS)[number]["id"];

export interface TopBarProps {
  active?: ActiveItem;
  onProfileClick?: () => void;
}

export function TopBar({ active = "search", onProfileClick }: TopBarProps) {
  return (
    <header className="fixed top-0 left-0 right-0 z-50 w-full border-b border-border bg-background">
      <div className="mx-auto flex h-14 max-w-[920px] items-center justify-between px-4 sm:px-6">
        <div className="flex items-center gap-4">
          <Link
            to={PATHS.home}
            className="flex items-center gap-2 font-mono text-sm font-medium tracking-tight text-foreground transition-colors hover:text-primary"
          >
            <span>cosine.search</span>
            <span className="font-mono text-xs text-muted-foreground">v1.0.4</span>
          </Link>
        </div>

        <div className="flex items-center gap-4">
          <nav aria-label="Main" className="flex items-center gap-2 sm:gap-4">
            {NAV_ITEMS.map((item) => {
              const isActive = item.id === active;

              return (
                <Button
                  key={item.id}
                  asChild
                  variant="ghost"
                  size="sm"
                  className={cn(
                    isActive ? "text-primary font-semibold" : "text-muted-foreground",
                  )}
                >
                  <Link to={item.to} aria-current={isActive ? "page" : undefined}>
                    {item.label}
                  </Link>
                </Button>
              );
            })}
          </nav>

          <div className="hidden h-4 w-px bg-border sm:block" />

          <Button
            type="button"
            size="icon-sm"
            aria-label="User Profile"
            onClick={onProfileClick}
            className="rounded-full"
          >
            <UserRound aria-hidden="true" className="size-4" />
          </Button>
        </div>
      </div>
    </header>
  );
}
