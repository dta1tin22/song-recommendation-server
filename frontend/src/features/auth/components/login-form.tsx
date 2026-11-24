import { ArrowRight, Eye, EyeOff } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";

import { PATHS } from "@/app/routes/paths";
import { Button } from "@/components/ui/button";

export function LoginForm() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [remember, setRemember] = useState(false);

  return (
    <div className="w-full max-w-[420px] rounded-lg border border-border bg-background p-7 shadow-sm">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">Log In</h1>
      </div>

      <form className="space-y-4" onSubmit={(event) => event.preventDefault()}>
        <div className="space-y-1.5">
          <label className="block text-sm font-medium text-foreground" htmlFor="email">
            Email
          </label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="name@archive.audio"
            className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label className="block text-sm font-medium text-foreground" htmlFor="password">
              Password
            </label>
            <a className="text-sm font-medium text-primary hover:underline" href="#">
              Forgot password?
            </a>
          </div>
          <div className="relative flex items-center">
            <input
              id="password"
              name="password"
              type={showPassword ? "text" : "password"}
              autoComplete="current-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="••••••••••••"
              className="h-10 w-full rounded-md border border-input bg-background pl-3 pr-10 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={showPassword ? "Hide password" : "Show password"}
              className="absolute right-1.5 text-muted-foreground"
              onClick={() => setShowPassword((visible) => !visible)}
            >
              {showPassword ? (
                <EyeOff aria-hidden="true" className="size-4" />
              ) : (
                <Eye aria-hidden="true" className="size-4" />
              )}
            </Button>
          </div>
        </div>

        <div className="flex items-center pt-1">
          <label
            className="flex cursor-pointer select-none items-center gap-2"
            htmlFor="remember"
          >
            <input
              id="remember"
              name="remember"
              type="checkbox"
              checked={remember}
              onChange={(event) => setRemember(event.target.checked)}
              className="size-3.5 cursor-pointer accent-primary"
            />
            <span className="text-sm text-muted-foreground">Keep session persistent</span>
          </label>
        </div>

        <div className="pt-2">
          <Button type="submit" className="w-full gap-2">
            Log In
            <ArrowRight aria-hidden="true" className="size-4" />
          </Button>
        </div>
      </form>

      <div className="mt-6 border-t border-border pt-5 text-center">
        <p className="text-sm text-muted-foreground">
          Don&apos;t have an account?
          <Link className="ml-1 font-medium text-primary hover:underline" to={PATHS.signup}>
            Sign up
          </Link>
        </p>
      </div>
    </div>
  );
}
