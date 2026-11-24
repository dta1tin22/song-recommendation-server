import { CodeXml, MessageCircle, Send } from "lucide-react";

import { Button } from "@/components/ui/button";

const SOCIAL_LINKS = [
  { label: "GitHub", href: "#", icon: CodeXml },
  { label: "X / Twitter", href: "#", icon: Send },
  { label: "Discord", href: "#", icon: MessageCircle },
] as const;

export function BottomBar() {
  return (
    <footer className="w-full border-t border-border bg-background">
      <div className="mx-auto flex h-14 max-w-[920px] items-center justify-between px-4 sm:px-6">
        <span className="font-mono text-sm font-medium tracking-tight text-foreground">
          cosine.search
        </span>

        <div className="flex items-center gap-4">
          {SOCIAL_LINKS.map(({ label, href, icon: Icon }) => (
            <Button key={label} asChild variant="ghost" size="icon-sm">
              <a href={href} aria-label={label}>
                <Icon aria-hidden="true" className="size-4" />
              </a>
            </Button>
          ))}
        </div>
      </div>
    </footer>
  );
}
